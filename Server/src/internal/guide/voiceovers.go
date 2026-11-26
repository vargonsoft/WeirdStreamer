package guide

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/weirdstreamer/server/internal/database"
	bgjobs "github.com/weirdstreamer/server/internal/jobs"
)

// The voice-overs made ahead: every 10 minutes (and after each planning), those of the programs
// in the next 4 hours on the library channels, so each is ready before its credits.

const (
	voiceOversAhead = 4 * time.Hour
	voiceOversEvery = 10 * time.Minute
	laterWithin     = 3 * time.Hour // a program after the next one, named in the line ("And at 9, …")
)

// VoiceOverPlan is what's said over a program's credits, and by whom.
type VoiceOverPlan struct {
	Announcer Announcer
	Line      string
}

// voiceOverPlan is the voice-over for slots[i] (a program): from the programs after it in slots.
func voiceOverPlan(f *VoicesFile, channel int, slots []database.GuideSlot, i int) (VoiceOverPlan, bool) {
	if f == nil || i >= len(slots) || slots[i].Kind != SlotProgram {
		return VoiceOverPlan{}, false
	}
	if info := slotInfo(slots[i]); info != nil && info.Block {
		return VoiceOverPlan{}, false // a block of clips: its next clip follows, not the next program
	}
	var next, later *ProgramInfo
	var laterStart, nextStart time.Time
	for j := i + 1; j < len(slots); j++ {
		if slots[j].Kind != SlotProgram {
			continue
		}
		info := slotInfo(slots[j])
		if info == nil {
			continue
		}
		if next == nil {
			next, nextStart = info, slots[j].Start
			continue
		}
		if slots[j].Start.Sub(nextStart) <= laterWithin && info.Title != next.Title {
			later, laterStart = info, slots[j].Start
		}
		break
	}
	line, ok := f.VoiceOverLine(channel, slots[i].Start, next, later, laterStart)
	if !ok {
		return VoiceOverPlan{}, false
	}
	return VoiceOverPlan{Announcer: f.AnnouncerFor(channel), Line: line}, true
}

func slotInfo(s database.GuideSlot) *ProgramInfo {
	if s.Info == "" {
		return nil
	}
	var p ProgramInfo
	if json.Unmarshal([]byte(s.Info), &p) != nil {
		return nil
	}
	return &p
}

// VoiceOverFor is a program's voice-over if it's been made: slots are its channel's, from it on.
func (s *Service) VoiceOverFor(channel int, slots []database.GuideSlot, i int) (VoiceOver, bool) {
	if s == nil || s.voices == nil || !s.voices.Enabled() {
		return VoiceOver{}, false
	}
	f, err := s.voices.File()
	if err != nil {
		return VoiceOver{}, false
	}
	plan, ok := voiceOverPlan(f, channel, slots, i)
	if !ok {
		return VoiceOver{}, false
	}
	return s.voices.Made(plan.Announcer, plan.Line)
}

// Voices is the voice-overs' maker (the tester's).
func (s *Service) Voices() *Voices { return s.voices }

// makeVoiceOvers makes the voice-overs of the programs ahead.
func (s *Service) makeVoiceOvers(ctx context.Context) {
	if !s.voices.Enabled() {
		return
	}
	f, err := s.voices.File()
	if err != nil {
		log.Printf("[VOICES] %v", err)
		return
	}
	chans, err := s.db.GuideChannels()
	if err != nil {
		return
	}
	now := time.Now()
	type todo struct {
		plan VoiceOverPlan
	}
	var work []todo
	for _, c := range chans {
		if !c.Active || c.LibraryID == "" {
			continue
		}
		slots, err := s.db.GuideSlots(c.Key, now, now.Add(voiceOversAhead+laterWithin+6*time.Hour), "")
		if err != nil {
			continue
		}
		for i, sl := range slots {
			if sl.Kind != SlotProgram || sl.Start.After(now.Add(voiceOversAhead)) {
				continue
			}
			if plan, ok := voiceOverPlan(f, c.Number, slots, i); ok {
				if _, made := s.voices.Made(plan.Announcer, plan.Line); !made {
					work = append(work, todo{plan})
				}
			}
		}
	}
	if len(work) == 0 {
		return
	}
	s.voiceJob.Begin(len(work))
	failed := 0
	for _, w := range work {
		if ctx.Err() != nil {
			return
		}
		s.voiceJob.SetDetail(w.plan.Line)
		if _, err := s.voices.Make(ctx, w.plan.Announcer, w.plan.Line, true); err != nil {
			failed++
			if failed == 1 {
				log.Printf("[VOICES] %q: %v", w.plan.Line, err)
			}
		}
		s.voiceJob.Step(1)
	}
	s.voiceJob.End(fmt.Sprintf("%d made, %d failed", len(work)-failed, failed))
}

func newVoiceJob() *bgjobs.Job {
	return bgjobs.Register("voiceovers", "Guide voice-overs",
		"Makes the announcer's \"Stay tuned\" voice-overs for the Guide's programs in the next 4 hours (Piper).")
}

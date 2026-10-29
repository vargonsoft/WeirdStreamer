// Package jobs keeps the background jobs' status for Settings → Jobs (what each is doing, how far
// along, when it last ran and when it runs next) and their controls: Run now, and Pause, which
// holds a job at its next step until resumed (not kept across restarts).
package jobs

import (
	"context"
	"log"
	"sort"
	"sync"
	"time"
)

// States a job can be in.
const (
	Idle    = "idle"    // between runs
	Running = "running" // working
	Waiting = "waiting" // ready to work, but the machine is busy (Note says why)
	Paused  = "paused"  // held by an admin
)

// Job is one background job. Its methods are safe to call from any goroutine.
type Job struct {
	ID    string
	Name  string
	About string // what it does, for the Jobs page
	order int

	mu         sync.Mutex
	state      string
	note       string
	detail     string   // how it's running, shown while it runs ("2 of 3 runners")
	runners    []string // what each runner is working on ("": nothing), for jobs that run several
	waiting    []bool   // runners stopped part way, waiting for a free one
	done       int
	total      int
	started    time.Time
	lastStart  time.Time
	lastEnd    time.Time
	lastResult string
	next       time.Time
	schedule   string // when it runs, when it has no next time ("On Scan Media")
	paused     bool
	resumed    chan struct{} // closed on Resume
	runNow     func()
}

var (
	mu   sync.Mutex
	list []*Job
	byID = map[string]*Job{}
)

// Register adds a job (or returns the one with that ID), listed in the order registered.
func Register(id, name, about string) *Job {
	mu.Lock()
	defer mu.Unlock()
	if j := byID[id]; j != nil {
		return j
	}
	j := &Job{ID: id, Name: name, About: about, order: len(list), state: Idle, resumed: make(chan struct{})}
	list = append(list, j)
	byID[id] = j
	return j
}

// Get returns a registered job, or nil.
func Get(id string) *Job {
	mu.Lock()
	defer mu.Unlock()
	return byID[id]
}

// OnRunNow sets what Run now does (usually waking the job).
func (j *Job) OnRunNow(f func()) {
	j.mu.Lock()
	j.runNow = f
	j.mu.Unlock()
}

// RunNow starts the job now, if it can be (false when it has no Run now).
func (j *Job) RunNow() bool {
	j.mu.Lock()
	f := j.runNow
	j.mu.Unlock()
	if f == nil {
		return false
	}
	f()
	return true
}

// Schedule describes when a job without a timer runs ("On Scan Media and server start").
func (j *Job) Schedule(s string) {
	j.mu.Lock()
	j.schedule = s
	j.mu.Unlock()
}

// Begin marks the start of a run of total steps (0 when unknown).
func (j *Job) Begin(total int) {
	j.mu.Lock()
	defer j.mu.Unlock()
	log.Printf("jobs: %s started", j.Name)
	j.started, j.done, j.total, j.next = time.Now(), 0, total, time.Time{}
	if !j.paused {
		j.state, j.note = Running, ""
	}
}

// SetTotal changes how many steps this run has.
func (j *Job) SetTotal(total int) {
	j.mu.Lock()
	j.total = total
	j.mu.Unlock()
}

// Step counts n steps done.
func (j *Job) Step(n int) {
	j.mu.Lock()
	j.done += n
	j.mu.Unlock()
}

// End marks the end of a run, with what it did ("Forgot 3 files").
func (j *Job) End(result string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	took := ""
	if !j.started.IsZero() {
		took = " in " + time.Since(j.started).Round(time.Second).String()
	}
	log.Printf("jobs: %s finished%s: %s", j.Name, took, result)
	j.lastStart, j.lastEnd, j.lastResult = j.started, time.Now(), result
	j.started, j.done, j.total, j.detail, j.runners, j.waiting = time.Time{}, 0, 0, "", nil, nil
	if !j.paused {
		j.state, j.note = Idle, ""
	}
}

// SetRunner says what runner i is working on ("" when nothing), for jobs that run several at
// once; cleared at the end of the run.
func (j *Job) SetRunner(i int, working string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for len(j.runners) <= i {
		j.runners = append(j.runners, "")
		j.waiting = append(j.waiting, false)
	}
	j.runners[i], j.waiting[i] = working, false
}

// RunnerWaiting marks runner i as stopped part way through its file, waiting for a free runner.
func (j *Job) RunnerWaiting(i int, waiting bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if i < len(j.waiting) {
		j.waiting[i] = waiting
	}
}

// runnerList is what each runner is on, for Status (with j.mu held).
func (j *Job) runnerList() []string {
	out := make([]string, len(j.runners))
	for i, f := range j.runners {
		out[i] = f
		if f != "" && j.waiting[i] {
			out[i] = f + " (waiting for a free runner)"
		}
	}
	return out
}

// SetDetail says how the run is going, for the Jobs page ("2 of 3 runners"); cleared at the end.
func (j *Job) SetDetail(detail string) {
	j.mu.Lock()
	j.detail = detail
	j.mu.Unlock()
}

// NextIn records when the job runs next by its timer.
func (j *Job) NextIn(d time.Duration) {
	j.mu.Lock()
	j.next = time.Now().Add(d)
	j.mu.Unlock()
}

// Pause holds the job at its next step.
func (j *Job) Pause() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.paused {
		j.paused = true
		j.state, j.note = Paused, ""
	}
}

// Resume lets a paused job go on.
func (j *Job) Resume() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.paused {
		j.paused = false
		close(j.resumed)
		j.resumed = make(chan struct{})
		if j.started.IsZero() {
			j.state = Idle
		} else {
			j.state = Running
		}
	}
}

// IsPaused says whether an admin has paused the job.
func (j *Job) IsPaused() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.paused
}

// Wait is called before each step: it holds the job while it's paused, and, when busy is given,
// while busy says the machine is too busy (why names the reason for the Jobs page). False when
// ctx ends first.
func (j *Job) Wait(ctx context.Context, busy func() (bool, string)) bool {
	for {
		j.mu.Lock()
		paused, resumed := j.paused, j.resumed
		j.mu.Unlock()
		if paused {
			select {
			case <-ctx.Done():
				return false
			case <-resumed:
			}
			continue
		}
		if busy != nil {
			if b, why := busy(); b {
				j.setWaiting(why)
				select {
				case <-ctx.Done():
					return false
				case <-resumed: // a pause or resume rechecks at once
				case <-time.After(30 * time.Second):
				}
				continue
			}
		}
		j.mu.Lock()
		if !j.paused && j.state == Waiting {
			j.state, j.note = Running, ""
		}
		j.mu.Unlock()
		return ctx.Err() == nil
	}
}

func (j *Job) setWaiting(why string) {
	j.mu.Lock()
	if !j.paused {
		j.state, j.note = Waiting, why
	}
	j.mu.Unlock()
}

// Status is a job as the Jobs page shows it. Times are absent when unknown.
type Status struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	About      string     `json:"about"`
	State      string     `json:"state"`
	Note       string     `json:"note,omitempty"`
	Detail     string     `json:"detail,omitempty"`
	Runners    []string   `json:"runners,omitempty"` // what each runner is working on ("": nothing)
	Done       int        `json:"done"`
	Total      int        `json:"total"`
	Started    *time.Time `json:"started,omitempty"`
	LastStart  *time.Time `json:"last_start,omitempty"`
	LastEnd    *time.Time `json:"last_end,omitempty"`
	LastResult string     `json:"last_result,omitempty"`
	Next       *time.Time `json:"next,omitempty"`
	Schedule   string     `json:"schedule,omitempty"`
	Paused     bool       `json:"paused"`
	CanRun     bool       `json:"can_run"`
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// All returns every job's status, in the order registered.
func All() []Status {
	mu.Lock()
	jobs := append([]*Job(nil), list...)
	mu.Unlock()
	sort.Slice(jobs, func(a, b int) bool { return jobs[a].order < jobs[b].order })
	out := make([]Status, 0, len(jobs))
	for _, j := range jobs {
		j.mu.Lock()
		out = append(out, Status{
			ID: j.ID, Name: j.Name, About: j.About, State: j.state, Note: j.note, Detail: j.detail, Runners: j.runnerList(),
			Done: j.done, Total: j.total, Started: timePtr(j.started),
			LastStart: timePtr(j.lastStart), LastEnd: timePtr(j.lastEnd), LastResult: j.lastResult,
			Next: timePtr(j.next), Schedule: j.schedule, Paused: j.paused, CanRun: j.runNow != nil,
		})
		j.mu.Unlock()
	}
	return out
}

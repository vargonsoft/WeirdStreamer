package handlers

import (
	"strconv"
	"testing"
	"time"

	"github.com/weirdstreamer/server/internal/models"
)

func TestSortNewAdditions(t *testing.T) {
	episode := func(show string, season, number int, at time.Time) continueItem {
		return continueItem{AddedAt: at.Unix(), MediaItem: models.MediaItem{Name: show,
			Metadata: &models.ItemMetadata{Kind: "episode", ShowTitle: show, Season: &season, Episode: &number}}}
	}
	day := time.Date(2026, 10, 28, 12, 0, 0, 0, time.Local)
	items := []continueItem{
		episode("Love is Blind", 11, 7, day.Add(-7*24*time.Hour)),
		episode("Love is Blind", 11, 9, day.Add(time.Hour)),
		episode("Love is Blind", 11, 10, day),
		episode("Love is Blind", 11, 11, day.Add(-time.Hour)),
		{AddedAt: day.Unix(), MediaItem: models.MediaItem{Name: "A Movie.mkv"}},
		episode("Love is Blind", 10, 12, day.Add(-2*time.Hour)),
	}
	sortNewAdditions(items)
	want := []string{"", "10x12", "11x9", "11x10", "11x11", "11x7"}
	for i, item := range items {
		got := ""
		if md := item.Metadata; md != nil {
			got = strconv.Itoa(*md.Season) + "x" + strconv.Itoa(*md.Episode)
		}
		if got != want[i] {
			t.Errorf("%d: %q, want %q", i, got, want[i])
		}
	}
}

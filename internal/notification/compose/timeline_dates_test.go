package compose

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
)

func timelineOf(events ...incident.Event) incident.Incident {
	return incident.Incident{Timeline: events}
}

// One day's lines keep the short clock; lines that span days carry the
// date, and the same minute on another day is its own line.
func TestTimelineDatesLinesOnlyWhenTheySpanDays(t *testing.T) {
	day := time.Date(2026, 10, 12, 9, 50, 0, 0, time.UTC)
	text := "Is scaled to 0 (deployment staging/reviews)"

	sameDay := timeline(timelineOf(
		incident.Event{At: day, Text: "kwatch noticed this"},
		incident.Event{At: day.Add(time.Hour), Text: text}), 10)
	assert.Equal(t, []string{"09:50  kwatch noticed this",
		"10:50  " + text}, sameDay)

	twoDays := timeline(timelineOf(
		incident.Event{At: day, Text: text},
		incident.Event{At: day.Add(24 * time.Hour), Text: text}), 10)
	assert.Equal(t, []string{"12 Oct 09:50  " + text,
		"13 Oct 09:50  " + text}, twoDays,
		"the same minute a day later is not merged into one line")
}

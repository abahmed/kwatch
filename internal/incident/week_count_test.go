package incident

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// The recurrence note counts the occurrences History keeps, the count
// the dashboard shows, not every opening: openings whose failures moved
// to another incident are no time of their own. It names when the last
// time was, never the ID of an incident it forgets.
func TestManagerRecurrenceNoteUsesOneCountAndNoID(t *testing.T) {
	m := NewManager(Config{IDNonce: testNonce}, &stubExplainer{})
	now := at(48 * time.Hour)
	var openings []time.Time
	for i := range 20 {
		openings = append(openings, at(time.Duration(i)*time.Hour))
	}
	previous := &Incident{ID: "inc-old", Root: podSig("web").Entity,
		State: Resolved, Opened: at(40 * time.Hour),
		Resolved: at(41 * time.Hour), Occurrences: openings,
		History: []Occurrence{{Opened: at(time.Hour),
			Resolved: at(2 * time.Hour)}}}
	p := &Incident{ID: "inc-new", Root: previous.Root, Opened: now}

	m.recur(p, previous, now)

	note := p.Timeline[len(p.Timeline)-1].Text
	assert.Equal(t, "happened again (3rd time this week, last time "+
		"Wed 04:00 UTC)", note)
	assert.False(t, strings.Contains(note, "inc-"))
	assert.Equal(t, 3, p.WeekCount(now))
}

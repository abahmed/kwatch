package inventory

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Notes of one reason with different topics, such as a container's
// readiness and liveness probe failures, keep their own message and
// count; one topic again replaces its own note.
func TestNoteTopicsAreKeptApart(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("order", "ops", "web")
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	note := func(topic, message string, count int, origin string) {
		m.Apply(Observation{Kind: Noted, Source: "test", At: at, Entity: id,
			Note: Note{At: at, Source: "kubelet", Reason: "Unhealthy",
				Topic: topic, Message: message, Count: count,
				Origin: origin}})
		at = at.Add(time.Minute)
	}
	note("readiness", "Readiness probe failed: refused", 23, "web.r")
	note("liveness", "Liveness probe failed: refused", 4, "web.l")
	note("readiness", "Readiness probe failed: refused again", 25, "web.r")

	notes := m.Notes(id, time.Time{})
	require.Len(t, notes, 2)
	byTopic := map[string]Note{}
	for _, n := range notes {
		byTopic[n.Topic] = n
	}
	assert.Equal(t, 25, byTopic["readiness"].Count)
	assert.Equal(t, "Readiness probe failed: refused again",
		byTopic["readiness"].Message)
	assert.Equal(t, 4, byTopic["liveness"].Count)
}

func TestNoteTopicSurvivesRecordedLog(t *testing.T) {
	in := Observation{Kind: Noted, Entity: CoreID("pod", "ns", "p"),
		Note: Note{Reason: "Unhealthy", Topic: "readiness"}}
	data, err := json.Marshal(in)
	require.NoError(t, err)
	var out Observation
	require.NoError(t, json.Unmarshal(data, &out))
	assert.Equal(t, "readiness", out.Note.Topic)
}

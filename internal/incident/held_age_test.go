package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRefreshAgeStatesTheAgeAtSendTime(t *testing.T) {
	since := time.Date(2026, 10, 7, 18, 3, 0, 0, time.UTC)
	now := since.Add(13*time.Hour + 9*time.Minute)
	cases := map[string]struct{ in, want string }{
		"node": {"Node stopped reporting (kubelet unreachable) for 2m",
			"Node stopped reporting (kubelet unreachable) for 13h9m"},
		"pending pod": {"cannot be scheduled (Unschedulable) for 5m30s",
			"cannot be scheduled (Unschedulable) for 13h9m"},
		"no age":      {"keeps crashing", "keeps crashing"},
		"not an age":  {"waits for 20h to retry", "waits for 20h to retry"},
		"no duration": {"waits for the volume", "waits for the volume"},
		"not ready": {"Node is NotReady for 2m", "Node is NotReady " +
			"for 13h9m"},
	}
	for name, c := range cases {
		assert.Equal(t, c.want, refreshAge(c.in, since, now), name)
	}
	assert.Equal(t, "is NotReady for 2m",
		refreshAge("is NotReady for 2m", time.Time{}, now),
		"a finding without an onset keeps its words")
}

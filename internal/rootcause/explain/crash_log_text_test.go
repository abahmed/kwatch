package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// logLineCase crashes both api containers with a termination message
// that says nothing, while their previous log's first error line names
// the cache Service they could not reach.
func logLineCase(f *fixture) inventory.EntityID {
	_, api, _ := redisFixture(f, 0, 0)
	for _, pod := range api {
		f.failError(containerOf(pod), "CrashLoop", "Error")
		f.observe(containerOf(pod), map[string]inventory.Value{
			kube.AttrLastErrorLine: inventory.Text(refusedRedis)})
	}
	return containerOf(api[0])
}

// A crash whose termination message says nothing is still explained by
// the previous log line that names the Service it could not call.
func TestPreviousLogLineNamesTheCalledService(t *testing.T) {
	f := newFixture(t)

	requireRoot(t, f, logLineCase(f), "service/shop/redis")
}

// The line is read only for what the container's own findings do not
// quote already.
func TestPreviousLogLineIsNotReadTwice(t *testing.T) {
	f := newFixture(t)
	effect := logLineCase(f)
	f.failError(effect, "Restarting", refusedRedis)
	v := newView(f.snapshot())

	if line := v.previousLogLine(effect); line != "" {
		t.Fatalf("a quoted line read again: %q", line)
	}
}

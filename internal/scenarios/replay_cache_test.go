package scenarios

import (
	"context"
	"sync"
	"testing"

	"github.com/abahmed/kwatch/internal/replay"
)

// Many tests judge the same committed scenario: the gates, the golden
// messages, the readability check and the parity checks. A replay is
// deterministic and tests only read its result, so each scenario is
// replayed once per test binary and every test shares that result.
// Treat a cached result as read-only.

// replayed is one scenario and what its replay produced.
type replayed struct {
	once   sync.Once
	log    replay.Log
	expect expectation
	result replay.Result
	err    error
}

var (
	replayedMu sync.Mutex
	replays    = map[string]*replayed{}
)

// replayScenario returns the committed scenario called name in dir, its
// label and its replay under the options its label asks for. Safe to
// call from parallel tests.
func replayScenario(
	t testing.TB, dir, name string,
) (replay.Log, expectation, replay.Result) {
	t.Helper()
	key := dir + "/" + name
	replayedMu.Lock()
	entry, ok := replays[key]
	if !ok {
		entry = &replayed{}
		replays[key] = entry
	}
	replayedMu.Unlock()
	entry.once.Do(func() {
		entry.log, entry.expect, entry.err = readScenarioFrom(dir, name)
		if entry.err != nil {
			return
		}
		entry.result, entry.err = replay.Run(context.Background(),
			entry.log, newDependencies(), entry.expect.options(entry.log.Start))
	})
	if entry.err != nil {
		t.Fatal(entry.err)
	}
	return entry.log, entry.expect, entry.result
}

// runParallelUnlessUpdating lets a test run beside the others. With
// -update the tests rewrite committed files, so they stay in order.
func runParallelUnlessUpdating(t *testing.T) {
	t.Helper()
	if !*update {
		t.Parallel()
	}
}

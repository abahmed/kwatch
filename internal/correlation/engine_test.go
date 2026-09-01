package correlation

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func mockClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func newTestEngine() *Engine {
	return NewEngine(Config{
		Window: 10 * time.Minute,
	})
}

func TestNewEngine(t *testing.T) {
	e := newTestEngine()
	assert.NotNil(t, e)
	assert.NotNil(t, e.state)
}

func TestProcessCreateNew(t *testing.T) {
	e := newTestEngine()

	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}
	owner := "deploy-1"

	inc, action := e.Process(ev, owner, nil)

	assert.Equal(t, model.ActionCreate, action)
	assert.NotNil(t, inc)
	assert.Equal(t, "default:deploy-1:CrashLoopBackOff:", string(inc.Key))
	assert.Equal(t, "deploy-1", inc.Name)
	assert.Equal(t, "default", inc.Namespace)
	assert.Equal(t, "CrashLoopBackOff", inc.Reason)
	assert.Equal(t, 1, inc.Count)
	assert.Equal(t, 1, len(inc.Resources))
	assert.True(t, inc.Resources["pod-1"])
	assert.True(t, inc.FirstSeen.Equal(inc.LastSeen))
}

func TestProcessRepeatedEventSkipsSameSig(t *testing.T) {
	e := newTestEngine()
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}

	// First event creates
	inc1, action1 := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action1)

	// Second event with identical sig → skip (edge-triggered), but Count
	// still updates
	ev.PodName = "pod-2"
	inc2, action2 := e.Process(ev, "deploy-1", nil)

	assert.Equal(t, model.ActionSkip, action2)
	assert.Equal(t, inc1.Key, inc2.Key)
	assert.Equal(t, 2, inc2.Count)
	assert.True(t, inc2.Resources["pod-1"])
	assert.True(t, inc2.Resources["pod-2"])
}

func TestProcessSkipSameSigSkipsButUpdatesCount(t *testing.T) {
	e := newTestEngine()
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}

	inc1, action1 := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action1)

	// Second event with same sig → skip (edge-triggered), Count and Resources
	// still update
	ev.PodName = "pod-2"
	inc2, action2 := e.Process(ev, "deploy-1", nil)

	assert.Equal(t, model.ActionSkip, action2)
	assert.Equal(t, inc1.Key, inc2.Key)
	assert.Equal(t, 2, inc2.Count)
	assert.True(t, inc2.Resources["pod-1"])
	assert.True(t, inc2.Resources["pod-2"])
}

func TestProcessDifferentOwnerNewIncident(t *testing.T) {
	e := newTestEngine()
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}

	_, action1 := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action1)

	// Same namespace+reason but different owner
	_, action2 := e.Process(ev, "deploy-2", nil)
	assert.Equal(t, model.ActionCreate, action2)
}

func TestProcessDifferentReasonNewIncident(t *testing.T) {
	e := newTestEngine()
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}

	_, action1 := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action1)

	// Same namespace+owner but different reason
	ev.Reason = "OOMKilled"
	_, action2 := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action2)
}

func TestProcessDifferentNamespaceNewIncident(t *testing.T) {
	e := newTestEngine()
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}

	_, action1 := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action1)

	// Different namespace
	ev.Namespace = "kube-system"
	_, action2 := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action2)
}

func TestProcessEmptyOwner(t *testing.T) {
	e := newTestEngine()
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "OOMKilled",
	}

	inc, action := e.Process(ev, "", nil)
	assert.Equal(t, model.ActionCreate, action)
	assert.Equal(t, "default::OOMKilled:", string(inc.Key))
}

func TestCleanup(t *testing.T) {
	e := newTestEngine()
	e.config.Window = 1 * time.Millisecond

	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}

	e.Process(ev, "deploy-1", nil)
	assert.Equal(t, 1, len(e.state))

	time.Sleep(2 * time.Millisecond)
	e.cleanup()
	assert.Equal(t, 0, len(e.state))
}

func TestCleanupKeepsRecent(t *testing.T) {
	e := newTestEngine()
	e.config.Window = 1 * time.Hour

	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}

	e.Process(ev, "deploy-1", nil)
	assert.Equal(t, 1, len(e.state))

	e.cleanup()
	assert.Equal(t, 1, len(e.state))
}

func TestRemovePodNoResolve(t *testing.T) {
	e := newTestEngine()

	ev1 := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}
	ev2 := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "OOMKilled",
	}

	e.Process(ev1, "deploy-1", nil)
	e.Process(ev2, "deploy-1", nil)

	assert.Equal(t, 2, len(e.state))

	// RemovePod must NOT resolve incidents — just remove the pod from resources
	e.RemovePod("default", "pod-1")

	assert.Equal(
		t,
		model.StateActive,
		e.state["default:deploy-1:CrashLoopBackOff:"].State,
		"incident must stay active after RemovePod",
	)
	assert.Equal(
		t,
		model.StateActive,
		e.state["default:deploy-1:OOMKilled:"].State,
		"incident must stay active after RemovePod",
	)
	assert.Equal(
		t,
		0,
		len(e.state["default:deploy-1:CrashLoopBackOff:"].Resources),
	)
	assert.Equal(t, 0, len(e.state["default:deploy-1:OOMKilled:"].Resources))
}

func TestProcessConcurrentSafe(t *testing.T) {
	e := newTestEngine()
	e.config.Window = time.Hour

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ev := event.Event{
				PodName:   "pod-1",
				Namespace: "default",
				Reason:    "CrashLoopBackOff",
			}
			e.Process(ev, "deploy-1", nil)
		}()
	}
	wg.Wait()

	assert.Equal(t, 1, len(e.state))
	inc := e.state["default:deploy-1:CrashLoopBackOff:"]
	assert.Equal(t, 100, inc.Count)
}

func TestBaselineSuppression(t *testing.T) {
	fakeNow := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newTestEngine()
	e.now = mockClock(fakeNow)

	incidentKey := BuildKey("default", "deploy-1", "CrashLoopBackOff", "")

	e.SetBaseline(
		map[string]map[string]int64{
			string(incidentKey): {"pod-1": fakeNow.Unix()},
		},
	)

	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}

	_, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionSkip, action)
}

func TestSetBaselineMergesNotReplaces(t *testing.T) {
	fakeNow := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newTestEngine()
	e.now = mockClock(fakeNow)

	key1 := BuildKey("default", "dep-1", "CrashLoopBackOff", "")
	key2 := BuildKey("default", "dep-1", "OOMKilled", "")
	key3 := BuildKey("default", "dep-2", "CrashLoopBackOff", "")

	// First call: seed key1 and key2
	e.SetBaseline(map[string]map[string]int64{
		string(key1): {"pod-a": fakeNow.Unix()},
		string(key2): {"pod-b": fakeNow.Unix()},
	})

	// Second call: same key1 with fresher timestamp, plus new key3
	later := fakeNow.Add(1 * time.Hour)
	e.SetBaseline(map[string]map[string]int64{
		string(key1): {"pod-a": later.Unix()},
		string(key3): {"pod-c": later.Unix()},
	})

	// All keys should be present (key1 and key2 preserved from first call,
	// key3 from second call, key1 timestamp updated)
	e.mu.Lock()
	defer e.mu.Unlock()

	_, ok1 := e.baseline[string(key1)]["pod-a"]
	assert.True(
		t,
		ok1,
		"key1 from first SetBaseline must survive after second SetBaseline",
	)

	_, ok2 := e.baseline[string(key2)]["pod-b"]
	assert.True(
		t,
		ok2,
		"key2 from first SetBaseline must survive after second SetBaseline "+
			"(merge)",
	)

	_, ok3 := e.baseline[string(key3)]["pod-c"]
	assert.True(t, ok3, "key3 from second SetBaseline must be present")

	// Timestamp for key1/pod-a must reflect the later value (was updated, not
	// stuck)
	assert.Equal(t, later.Unix(), e.baseline[string(key1)]["pod-a"],
		"SetBaseline must update timestamp for existing entry")
}

func TestClearSeenUnsuppresses(t *testing.T) {
	e := newTestEngine()

	incidentKey := BuildKey("default", "deploy-1", "CrashLoopBackOff", "")

	e.SetBaseline(
		map[string]map[string]int64{
			string(incidentKey): {"pod-1": time.Now().Unix()},
		},
	)
	e.ClearBaselineForPod("default", "pod-1")

	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}

	_, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)
}

package correlation

import (
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/stretchr/testify/assert"
)

func newTestEngine() *Engine {
	return NewEngine(Config{
		Window:   10 * time.Minute,
		Cooldown: 5 * time.Minute,
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

	inc, action := e.Process(ev, owner)

	assert.Equal(t, model.ActionCreate, action)
	assert.NotNil(t, inc)
	assert.Equal(t, "default:deploy-1:CrashLoopBackOff:", inc.Key)
	assert.Equal(t, "deploy-1", inc.Name)
	assert.Equal(t, "default", inc.Namespace)
	assert.Equal(t, "CrashLoopBackOff", inc.Reason)
	assert.Equal(t, 1, inc.Count)
	assert.Equal(t, 1, len(inc.Resources))
	assert.True(t, inc.Resources["pod-1"])
	assert.True(t, inc.FirstSeen.Equal(inc.LastSeen))
}

func TestProcessUpdateAfterCooldown(t *testing.T) {
	e := newTestEngine()
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}

	// First event creates
	inc1, action1 := e.Process(ev, "deploy-1")
	assert.Equal(t, model.ActionCreate, action1)

	// Wait for cooldown to pass
	time.Sleep(1 * time.Millisecond)
	e.config.Cooldown = 1 * time.Millisecond

	// Second event should update
	ev.PodName = "pod-2"
	inc2, action2 := e.Process(ev, "deploy-1")

	assert.Equal(t, model.ActionUpdate, action2)
	assert.Equal(t, inc1.Key, inc2.Key)
	assert.Equal(t, 2, inc2.Count)
	assert.True(t, inc2.Resources["pod-1"])
	assert.True(t, inc2.Resources["pod-2"])
}

func TestProcessSkipWithinCooldown(t *testing.T) {
	e := newTestEngine()
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}

	inc1, action1 := e.Process(ev, "deploy-1")
	assert.Equal(t, model.ActionCreate, action1)

	// Second event within cooldown (0 time passed)
	ev.PodName = "pod-2"
	inc2, action2 := e.Process(ev, "deploy-1")

	assert.Equal(t, model.ActionSkip, action2)
	assert.Equal(t, inc1.Key, inc2.Key)
	assert.Equal(t, 1, inc2.Count) // unchanged
	assert.True(t, inc2.Resources["pod-1"])
	assert.False(t, inc2.Resources["pod-2"])
}

func TestProcessDifferentOwnerNewIncident(t *testing.T) {
	e := newTestEngine()
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}

	_, action1 := e.Process(ev, "deploy-1")
	assert.Equal(t, model.ActionCreate, action1)

	// Same namespace+reason but different owner
	_, action2 := e.Process(ev, "deploy-2")
	assert.Equal(t, model.ActionCreate, action2)
}

func TestProcessDifferentReasonNewIncident(t *testing.T) {
	e := newTestEngine()
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}

	_, action1 := e.Process(ev, "deploy-1")
	assert.Equal(t, model.ActionCreate, action1)

	// Same namespace+owner but different reason
	ev.Reason = "OOMKilled"
	_, action2 := e.Process(ev, "deploy-1")
	assert.Equal(t, model.ActionCreate, action2)
}

func TestProcessDifferentNamespaceNewIncident(t *testing.T) {
	e := newTestEngine()
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}

	_, action1 := e.Process(ev, "deploy-1")
	assert.Equal(t, model.ActionCreate, action1)

	// Different namespace
	ev.Namespace = "kube-system"
	_, action2 := e.Process(ev, "deploy-1")
	assert.Equal(t, model.ActionCreate, action2)
}

func TestProcessEmptyOwner(t *testing.T) {
	e := newTestEngine()
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "OOMKilled",
	}

	inc, action := e.Process(ev, "")
	assert.Equal(t, model.ActionCreate, action)
	assert.Equal(t, "default::OOMKilled:", inc.Key)
}

func TestCleanup(t *testing.T) {
	e := newTestEngine()
	e.config.Window = 1 * time.Millisecond

	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
	}

	e.Process(ev, "deploy-1")
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

	e.Process(ev, "deploy-1")
	assert.Equal(t, 1, len(e.state))

	e.cleanup()
	assert.Equal(t, 1, len(e.state))
}

func TestProcessConcurrentSafe(t *testing.T) {
	e := newTestEngine()
	e.config.Cooldown = 0
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
			e.Process(ev, "deploy-1")
		}()
	}
	wg.Wait()

	assert.Equal(t, 1, len(e.state))
	inc := e.state["default:deploy-1:CrashLoopBackOff:"]
	assert.Equal(t, 100, inc.Count)
}

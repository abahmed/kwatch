package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func TestPerPodBaselineNewPodAlerts(t *testing.T) {
	e := newTestEngine()

	key := BuildKey("default", "deploy-1", "CrashLoopBackOff", "")
	e.SetBaseline(
		map[string]map[string]int64{string(key): {"pod-1": time.Now().Unix()}},
	)

	// pod-1 is baselined — should skip
	ev1 := event.Event{
		Namespace: "default",
		PodName:   "pod-1",
		Reason:    "CrashLoopBackOff",
	}
	_, action := e.processEvent(ev1, "deploy-1", nil)
	assert.Equal(t, model.ActionSkip, action)

	// pod-2 is new — should alert
	ev2 := event.Event{
		Namespace: "default",
		PodName:   "pod-2",
		Reason:    "CrashLoopBackOff",
	}
	_, action = e.processEvent(ev2, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)
}

func TestClearBaselineForPodIsPerPod(t *testing.T) {
	e := newTestEngine()

	key := BuildKey("default", "deploy-1", "CrashLoopBackOff", "")
	e.SetBaseline(
		map[string]map[string]int64{
			string(key): {
				"pod-1": time.Now().Unix(),
				"pod-2": time.Now().Unix(),
			},
		},
	)

	e.ClearBaselineForPod(
		"default", "pod-1",
		model.ObjectRef{Kind: "Deployment", Namespace: "default", Name: "deploy-1"},
	)

	// pod-1 un-baselined → create
	ev1 := event.Event{
		Namespace: "default",
		PodName:   "pod-1",
		Reason:    "CrashLoopBackOff",
	}
	_, action := e.processEvent(ev1, "deploy-1", nil)
	assert.Equal(t, model.ActionCreate, action)

	// pod-2 still baselined → skip
	ev2 := event.Event{
		Namespace: "default",
		PodName:   "pod-2",
		Reason:    "CrashLoopBackOff",
	}
	_, action = e.processEvent(ev2, "deploy-1", nil)
	assert.Equal(t, model.ActionSkip, action)
}

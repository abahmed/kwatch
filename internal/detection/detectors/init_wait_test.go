package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// initModel is pod api-1 whose init container wait-for-db started at t0.
// calls is the container's AttrServiceCalls ("" for none).
func initModel(calls string) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	pod := newID(kube.KindPod, "shop", "api-1")
	put(m, pod, t0, nil)
	c := newID(kube.KindContainer, "shop", "api-1/wait-for-db")
	attrs := map[string]inventory.Value{
		kube.AttrInit:      inventory.Bool(true),
		kube.AttrState:     inventory.Text("running"),
		kube.AttrStartedAt: inventory.Time(t0),
	}
	if calls != "" {
		attrs[kube.AttrServiceCalls] = inventory.Text(calls)
	}
	put(m, c, t0, attrs)
	link(m, c, inventory.PartOf, pod)
	return m, c
}

func TestInitWaitReportsAfterTheDefaultWithoutHistory(t *testing.T) {
	m, c := initModel("")
	early := evaluate(InitWait{}, m, t0.Add(2*time.Minute), c, nil)
	assert.Empty(t, early.Findings)
	assert.NotZero(t, early.RecheckAfter, "the clock alone can make it stuck")

	got := evaluate(InitWait{}, m, t0.Add(6*time.Minute), c, nil).Findings
	require.Len(t, got, 1)
	assert.Equal(t, reasons.InitContainerWaiting, got[0].Reason)
	assert.Equal(t, "is stuck in init: container wait-for-db has been "+
		"running for 6m", got[0].Summary)
}

func TestInitWaitNamesADependencyWithoutReadyEndpoints(t *testing.T) {
	m, c := initModel("shop/db:5432")
	svc := newID(kube.KindService, "shop", "db")
	put(m, svc, t0, map[string]inventory.Value{
		kube.AttrSelector: inventory.Text("app=db"),
	})
	slice := newID(kube.KindEndpointSlice, "shop", "db-1")
	put(m, slice, t0, map[string]inventory.Value{
		kube.AttrEndpoints:      inventory.Number(1),
		kube.AttrEndpointsReady: inventory.Number(0),
	})
	link(m, slice, inventory.Backs, svc)

	got := evaluate(InitWait{}, m, t0.Add(6*time.Minute), c, nil).Findings
	require.Len(t, got, 1)
	assert.Equal(t, "is stuck in init: container wait-for-db has been "+
		"waiting 6m for db:5432 (Service db in shop has no ready "+
		"endpoints)", got[0].Summary)
}

func TestInitWaitHealthyDependencyStatesFactsOnly(t *testing.T) {
	m, c := initModel("shop/db:5432")
	svc := newID(kube.KindService, "shop", "db")
	put(m, svc, t0, map[string]inventory.Value{
		kube.AttrSelector: inventory.Text("app=db"),
	})
	slice := newID(kube.KindEndpointSlice, "shop", "db-1")
	put(m, slice, t0, map[string]inventory.Value{
		kube.AttrEndpoints:      inventory.Number(1),
		kube.AttrEndpointsReady: inventory.Number(1),
	})
	link(m, slice, inventory.Backs, svc)

	got := evaluate(InitWait{}, m, t0.Add(6*time.Minute), c, nil).Findings
	require.Len(t, got, 1)
	assert.Equal(t, "is stuck in init: container wait-for-db has been "+
		"running for 6m", got[0].Summary)
}

func TestInitWaitMissingDependency(t *testing.T) {
	m, c := initModel("shop/db:5432")
	got := evaluate(InitWait{}, m, t0.Add(6*time.Minute), c, nil).Findings
	require.Len(t, got, 1)
	assert.Contains(t, got[0].Summary, "(Service db in shop does not exist)")
}

// A workload whose init usually takes ten minutes is not stuck at six,
// and one whose init usually takes seconds is stuck at two.
func TestInitWaitJudgesAgainstTheWorkloadsOwnHistory(t *testing.T) {
	learn := func(m *inventory.Model, seconds float64) {
		pod := newID(kube.KindPod, "shop", "api-1")
		for i := 0; i < 20; i++ {
			m.Baselines().Add(pod, inventory.MetricInitSeconds, seconds,
				t0.Add(-time.Duration(i+1)*time.Hour))
		}
	}
	m, c := initModel("")
	learn(m, 600)
	assert.Empty(t, evaluate(InitWait{}, m, t0.Add(6*time.Minute), c,
		nil).Findings)

	m, c = initModel("")
	learn(m, 5)
	got := evaluate(InitWait{}, m, t0.Add(2*time.Minute), c, nil).Findings
	require.Len(t, got, 1)
	assert.Contains(t, got[0].Evidence[0].Value, "vs a usual")
}

func TestInitWaitIgnoresOtherContainers(t *testing.T) {
	now := t0.Add(time.Hour)
	m, c := initModel("")
	put(m, c, t0, map[string]inventory.Value{
		kube.AttrInit:      inventory.Bool(false),
		kube.AttrState:     inventory.Text("running"),
		kube.AttrStartedAt: inventory.Time(t0)})
	assert.Empty(t, evaluate(InitWait{}, m, now, c, nil).Findings,
		"an app container or a sidecar is not an init wait")

	m, c = initModel("")
	put(m, c, t0, map[string]inventory.Value{
		kube.AttrInit:  inventory.Bool(true),
		kube.AttrState: inventory.Text("terminated")})
	assert.Empty(t, evaluate(InitWait{}, m, now, c, nil).Findings,
		"a finished init container is done")
	assert.Equal(t, "init-wait", InitWait{}.Name())
}

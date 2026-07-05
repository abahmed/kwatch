package handler

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDetectDaemonSetIssueUnavailable(t *testing.T) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ds1", Namespace: "ns1"},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 3,
			NumberUnavailable:      1,
		},
	}
	sig := DetectDaemonSetIssue(ds)
	assert.NotNil(t, sig)
	assert.Equal(t, "DaemonSetUnavailable", sig.Reason)
}

func TestDetectDaemonSetIssueNoIssue(t *testing.T) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ds1", Namespace: "ns1"},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 3,
			NumberUnavailable:      0,
		},
	}
	assert.Nil(t, DetectDaemonSetIssue(ds))
}

func TestAvailabilityHint(t *testing.T) {
	ds := &appsv1.DaemonSet{
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 5,
			NumberUnavailable:      2,
			NumberAvailable:        3,
		},
	}
	hint := availabilityHint(ds)
	assert.Contains(t, hint, "2/5")
}

func TestProcessDaemonSetObjectNil(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	assert.NoError(t, h.ProcessDaemonSetObject(nil, false))
}

func TestProcessDaemonSetObjectDeleted(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ds1", Namespace: "ns1"},
	}
	assert.NoError(t, h.ProcessDaemonSetObject(ds, true))
}

func TestProcessDaemonSetObjectHealthy(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ds1", Namespace: "ns1"},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 3,
			NumberAvailable:        3,
		},
	}
	assert.NoError(t, h.ProcessDaemonSetObject(ds, false))
	assert.Equal(t, 0, e.ActiveCount())
}

func TestProcessDaemonSetObjectUnavailable(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ds1", Namespace: "ns1"},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 3,
			NumberUnavailable:      1,
			UpdatedNumberScheduled: 3,
			ObservedGeneration:     1,
		},
	}
	ds.Generation = 1
	assert.NoError(t, h.ProcessDaemonSetObject(ds, false))
	assert.Equal(t, 1, e.ActiveCount())
}

func TestProcessDaemonSetInvalidKey(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	assert.Error(t, h.ProcessDaemonSet("a/b/c", false))
}

func TestProcessDaemonSetDeleted(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	assert.NoError(t, h.ProcessDaemonSet("ns1/ds1", true))
}

func TestProcessDaemonSetNotFound(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	client := fake.NewSimpleClientset()
	f := informers.NewSharedInformerFactory(client, 0)
	h.SetDaemonSetLister(f.Apps().V1().DaemonSets().Lister())
	assert.NoError(t, h.ProcessDaemonSet("ns1/missing", false))
}

func TestProcessDaemonSetObjectNodeInhibition(t *testing.T) {
	e := testCorrelator()
	e.SetActiveNodeIncidents([]string{"node1"})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ds1", Namespace: "ns1"},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 3,
			NumberUnavailable:      1,
			UpdatedNumberScheduled: 3,
			ObservedGeneration:     1,
		},
	}
	ds.Generation = 1
	assert.NoError(t, h.ProcessDaemonSetObject(ds, false))
	assert.Equal(t, 0, e.ActiveCount(), "should be suppressed by node inhibition")
}

func TestProcessDaemonSetObjectRolloutGrace(t *testing.T) {
	e := testCorrelator()
	now := time.Now()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	h.(*handler).now = func() time.Time { return now }

	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ds1", Namespace: "ns1"},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 3,
			NumberUnavailable:      1,
			UpdatedNumberScheduled: 1,
			ObservedGeneration:     0,
		},
	}
	ds.Generation = 1
	assert.NoError(t, h.ProcessDaemonSetObject(ds, false))
	assert.Equal(t, 0, e.ActiveCount(), "should be suppressed during rollout grace")
}

func TestProcessDaemonSetObjectSustained(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{
		DaemonSetMonitor: config.DaemonSetMonitor{SustainedMinutes: 10},
	}, e, testAlertMgr)

	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ds1", Namespace: "ns1"},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 3,
			NumberUnavailable:      1,
			UpdatedNumberScheduled: 3,
			ObservedGeneration:     1,
		},
	}
	ds.Generation = 1
	assert.NoError(t, h.ProcessDaemonSetObject(ds, false))
	assert.Equal(t, 0, e.ActiveCount(), "should be suppressed during sustained window")
}

func TestMarkFirstUnavailableDSHit(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr).(*handler)
	t1 := h.markFirstUnavailableDS("ns1/ds1")
	t2 := h.markFirstUnavailableDS("ns1/ds1")
	assert.Equal(t, t1, t2, "second call should return existing entry")
}

func TestProcessDaemonSetUnavailable(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ds1", Namespace: "ns1"},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 3,
			NumberUnavailable:      1,
			UpdatedNumberScheduled: 3,
			ObservedGeneration:     1,
		},
	}
	ds.Generation = 1
	client := fake.NewSimpleClientset()
	f := informers.NewSharedInformerFactory(client, 0)
	f.Apps().V1().DaemonSets().Informer().GetIndexer().Add(ds)
	h.SetDaemonSetLister(f.Apps().V1().DaemonSets().Lister())
	assert.NoError(t, h.ProcessDaemonSet("ns1/ds1", false))
	assert.Equal(t, 1, e.ActiveCount())
}

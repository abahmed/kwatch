package handler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
)

func TestDetectStatefulSetIssueUnavailable(t *testing.T) {
	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ss1", Namespace: "ns1"},
		Status:     appsv1.StatefulSetStatus{Replicas: 3, ReadyReplicas: 1},
	}
	sig := DetectStatefulSetIssue(ss)
	assert.NotNil(t, sig)
	assert.Equal(t, "StsUnavailable", sig.Reason)
}

func TestDetectStatefulSetIssueNoIssue(t *testing.T) {
	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ss1", Namespace: "ns1"},
		Status:     appsv1.StatefulSetStatus{Replicas: 3, ReadyReplicas: 3},
	}
	assert.Nil(t, DetectStatefulSetIssue(ss))
}

func TestStsAvailabilityHint(t *testing.T) {
	ss := &appsv1.StatefulSet{
		Status: appsv1.StatefulSetStatus{Replicas: 3, ReadyReplicas: 1},
	}
	hint := stsAvailabilityHint(ss)
	assert.Contains(t, hint, "2/3")
}

func TestProcessStatefulSetObjectNil(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)
	assert.NoError(t, h.ProcessStatefulSetObject(nil, false))
}

func TestProcessStatefulSetObjectDeleted(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)
	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ss1", Namespace: "ns1"},
	}
	assert.NoError(t, h.ProcessStatefulSetObject(ss, true))
}

func TestProcessStatefulSetObjectHealthy(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)
	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ss1", Namespace: "ns1"},
		Status:     appsv1.StatefulSetStatus{Replicas: 3, ReadyReplicas: 3},
	}
	assert.NoError(t, h.ProcessStatefulSetObject(ss, false))
	assert.Equal(t, 0, e.ActiveCount())
}

func TestProcessStatefulSetObjectUnavailable(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)
	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ss1", Namespace: "ns1"},
		Status: appsv1.StatefulSetStatus{
			Replicas:           3,
			ReadyReplicas:      1,
			CurrentReplicas:    3,
			ObservedGeneration: 1,
		},
	}
	ss.Generation = 1
	assert.NoError(t, h.ProcessStatefulSetObject(ss, false))
	assert.Equal(t, 1, e.ActiveCount())
}

func TestProcessStatefulSetObjectRolloutGrace(t *testing.T) {
	e := testCorrelator()
	now := time.Now()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)
	h.now = func() time.Time { return now }

	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ss1", Namespace: "ns1"},
		Status: appsv1.StatefulSetStatus{
			Replicas:           3,
			ReadyReplicas:      1,
			CurrentReplicas:    1,
			ObservedGeneration: 0,
		},
	}
	ss.Generation = 1
	assert.NoError(t, h.ProcessStatefulSetObject(ss, false))
	assert.Equal(
		t,
		0,
		e.ActiveCount(),
		"should be suppressed during rollout grace",
	)
}

func TestProcessStatefulSetObjectSustained(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{
		StatefulSetMonitor: config.StatefulSetMonitor{SustainedMinutes: 10},
	}, e, testAlertMgr)

	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ss1", Namespace: "ns1"},
		Status: appsv1.StatefulSetStatus{
			Replicas:           3,
			ReadyReplicas:      1,
			CurrentReplicas:    3,
			ObservedGeneration: 1,
		},
	}
	ss.Generation = 1
	assert.NoError(t, h.ProcessStatefulSetObject(ss, false))
	assert.Equal(
		t,
		0,
		e.ActiveCount(),
		"should be suppressed during sustained window",
	)
}

func TestMarkFirstUnavailableStsHit(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)
	t1 := h.markFirstUnavailableSts("ns1/ss1")
	t2 := h.markFirstUnavailableSts("ns1/ss1")
	assert.Equal(t, t1, t2, "second call should return existing entry")
}

func TestProcessStatefulSetInvalidKey(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	assert.Error(t, h.ProcessStatefulSet("a/b/c", false))
}

func TestProcessStatefulSetDeleted(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	assert.NoError(t, h.ProcessStatefulSet("ns1/ss1", true))
}

func TestProcessStatefulSetNotFound(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)
	client := fake.NewSimpleClientset()
	f := informers.NewSharedInformerFactory(client, 0)
	h.listers.SS = f.Apps().V1().StatefulSets().Lister()
	assert.NoError(t, h.ProcessStatefulSet("ns1/missing", false))
}

func TestProcessStatefulSetUnavailable(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)
	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ss1", Namespace: "ns1"},
		Status: appsv1.StatefulSetStatus{
			Replicas:           3,
			ReadyReplicas:      1,
			CurrentReplicas:    3,
			ObservedGeneration: 1,
		},
	}
	ss.Generation = 1
	client := fake.NewSimpleClientset()
	f := informers.NewSharedInformerFactory(client, 0)
	f.Apps().V1().StatefulSets().Informer().GetIndexer().Add(ss)
	h.listers.SS = f.Apps().V1().StatefulSets().Lister()
	assert.NoError(t, h.ProcessStatefulSet("ns1/ss1", false))
	assert.Equal(t, 1, e.ActiveCount())
}

package handler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
)

func TestIsPdbBlockingTrue(t *testing.T) {
	pdb := &policyv1.PodDisruptionBudget{
		Status: policyv1.PodDisruptionBudgetStatus{
			ObservedGeneration: 1,
			DesiredHealthy:     2,
			DisruptionsAllowed: 0,
			CurrentHealthy:     1,
		},
	}
	pdb.Generation = 1
	assert.True(t, isPdbBlocking(pdb))
}

func TestIsPdbBlockingFalseAllowed(t *testing.T) {
	pdb := &policyv1.PodDisruptionBudget{
		Status: policyv1.PodDisruptionBudgetStatus{
			ObservedGeneration: 1,
			DesiredHealthy:     2,
			DisruptionsAllowed: 1,
			CurrentHealthy:     2,
		},
	}
	pdb.Generation = 1
	assert.False(t, isPdbBlocking(pdb))
}

func TestDetectPdbIssue(t *testing.T) {
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "pdb1", Namespace: "ns1"},
		Status: policyv1.PodDisruptionBudgetStatus{
			ObservedGeneration: 1,
			DesiredHealthy:     2,
			DisruptionsAllowed: 0,
			CurrentHealthy:     1,
		},
	}
	pdb.Generation = 1
	sig := DetectPdbIssue(pdb)
	assert.NotNil(t, sig)
	assert.Equal(t, "PdbViolation", sig.Reason)
}

func TestDetectPdbIssueNoIssue(t *testing.T) {
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "pdb1", Namespace: "ns1"},
		Status: policyv1.PodDisruptionBudgetStatus{
			DesiredHealthy:     2,
			DisruptionsAllowed: 1,
		},
	}
	assert.Nil(t, DetectPdbIssue(pdb))
}

func TestPdbHint(t *testing.T) {
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "pdb1", Namespace: "ns1"},
		Status: policyv1.PodDisruptionBudgetStatus{
			CurrentHealthy: 1,
			DesiredHealthy: 2,
		},
	}
	hint := pdbHint(pdb)
	assert.Contains(t, hint, "pdb1")
	assert.Contains(t, hint, "desiredHealthy=2")
}

func TestProcessPdbObjectNil(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	assert.NoError(t, h.ProcessPdbObject(nil, false))
}

func TestProcessPdbObjectDeleted(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "pdb1", Namespace: "ns1"},
	}
	assert.NoError(t, h.ProcessPdbObject(pdb, true))
}

func TestProcessPdbObjectBlocking(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "pdb1", Namespace: "ns1"},
		Status: policyv1.PodDisruptionBudgetStatus{
			ObservedGeneration: 1,
			DesiredHealthy:     2,
			DisruptionsAllowed: 0,
			CurrentHealthy:     1,
		},
	}
	pdb.Generation = 1
	assert.NoError(t, h.ProcessPdbObject(pdb, false))
	assert.Equal(t, 1, e.ActiveCount())
}

func TestProcessPdbObjectHealthy(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "pdb1", Namespace: "ns1"},
		Status: policyv1.PodDisruptionBudgetStatus{
			ObservedGeneration: 1,
			DesiredHealthy:     2,
			DisruptionsAllowed: 1,
		},
	}
	pdb.Generation = 1
	assert.NoError(t, h.ProcessPdbObject(pdb, false))
	assert.Equal(t, 0, e.ActiveCount())
}

func TestProcessPdbObjectSustained(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{
		PdbMonitor: config.PdbMonitor{SustainedMinutes: 10},
	}, e, testAlertMgr)

	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "pdb1", Namespace: "ns1"},
		Status: policyv1.PodDisruptionBudgetStatus{
			ObservedGeneration: 1,
			DesiredHealthy:     2,
			DisruptionsAllowed: 0,
			CurrentHealthy:     1,
		},
	}
	pdb.Generation = 1
	assert.NoError(t, h.ProcessPdbObject(pdb, false))
	assert.Equal(t, 0, e.ActiveCount(), "should be suppressed during sustained window")
}

func TestProcessPdbObjectExistingEntry(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	hh := h

	// Seed an existing entry so markFirstPdbViolation returns the stored time
	hh.firstPdbViolation["ns1/pdb1"] = time.Now().Add(-1 * time.Hour)

	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "pdb1", Namespace: "ns1"},
		Status: policyv1.PodDisruptionBudgetStatus{
			ObservedGeneration: 1,
			DesiredHealthy:     2,
			DisruptionsAllowed: 0,
			CurrentHealthy:     1,
		},
	}
	pdb.Generation = 1
	assert.NoError(t, h.ProcessPdbObject(pdb, false))
	assert.Equal(t, 1, e.ActiveCount(), "existing entry with old timestamp should fire")
}

func TestProcessPdbInvalidKey(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	assert.Error(t, h.ProcessPdb("a/b/c", false))
}

func TestProcessPdbDeleted(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	assert.NoError(t, h.ProcessPdb("ns1/pdb1", true))
}

func TestProcessPdbNoop(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)

	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "pdb1", Namespace: "ns1"},
		Status: policyv1.PodDisruptionBudgetStatus{
			ObservedGeneration: 1,
			DesiredHealthy:     2,
			DisruptionsAllowed: 1,
			CurrentHealthy:     2,
		},
	}
	pdb.Generation = 1

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(pdb), 0)
	h.SetPdbLister(f.Policy().V1().PodDisruptionBudgets().Lister())

	assert.NoError(t, h.ProcessPdb("ns1/pdb1", false))
}

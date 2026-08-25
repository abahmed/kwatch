package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
)

func TestSetReplicaLister(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.SetReplicaLister(f.Apps().V1().ReplicaSets().Lister())
	assert.NotNil(t, h.listers.rs)
}

func TestSetSecretLister(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.SetSecretLister(f.Core().V1().Secrets().Lister())
	assert.NotNil(t, h.listers.secret)
}

func TestSetInsightEngine(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	h.SetInsightEngine(nil)
	assert.Nil(t, h.insightEngine)
}

func TestSetBaseline(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	h.SetBaseline(map[string]map[string]int64{"default/test-pod": {"CrashLoopBackOff": 100}})
}

func TestSetActiveNodeIncidents(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	h.SetActiveNodeIncidents([]string{"node1", "node2"})
}

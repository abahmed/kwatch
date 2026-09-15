package pod

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/monitor/pod/enrichment"
)

func TestRuntimeEvaluatesPodAndRunsRecovery(t *testing.T) {
	sink := &runtimeSink{}
	evaluator := &runtimeEvaluator{}
	runtime := NewRuntimeWithRuntimeConfig(
		nil, config.RuntimeConfig{}, sink, evaluator, nil, time.Now,
	)
	wantNow := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	runtime.now = func() time.Time { return wantNow }
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "apps", Name: "api", UID: "pod-1",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "api", Ready: true,
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{},
				},
			}},
		},
	}

	if err := runtime.ProcessPodObject(
		context.Background(), pod, false,
	); err != nil {
		t.Fatalf("ProcessPodObject() returned error: %v", err)
	}
	if !evaluator.pod || !evaluator.containers {
		t.Fatalf("runtime did not run both evaluator stages: %#v", evaluator)
	}
	if sink.cleared != 1 || sink.recovered != 1 {
		t.Fatalf("unexpected recovery calls: %#v", sink)
	}
}

func TestRuntimeIgnoresStalePodDeletion(t *testing.T) {
	sink := &runtimeSink{}
	runtime := NewRuntimeWithRuntimeConfig(
		nil, config.RuntimeConfig{}, sink, nil, nil, time.Now,
	)
	runtime.SetSources(RuntimeSources{Pod: newPodLister(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Namespace: "apps", Name: "api", UID: "new",
		}},
	)})

	if err := runtime.ProcessPod(
		context.Background(), "apps/api#old", true,
	); err != nil {
		t.Fatalf("ProcessPod() returned error: %v", err)
	}
	if sink.removed != 0 {
		t.Fatalf("stale deletion removed current Pod: %#v", sink)
	}
}

type runtimeEvaluator struct {
	pod        bool
	containers bool
}

func (e *runtimeEvaluator) EvaluatePod(*enrichment.Context) {
	e.pod = true
}

func (e *runtimeEvaluator) EvaluateContainers(*enrichment.Context) {
	e.containers = true
}

type runtimeSink struct {
	cleared   int
	recovered int
	removed   int
}

func (s *runtimeSink) SetBaseline(map[string]map[string]int64) {}

func (s *runtimeSink) SetActiveNodeIncidents([]string) {}

func (s *runtimeSink) Process(
	*model.Observation,
) (*model.Incident, model.IncidentAction) {
	return nil, model.ActionSkip
}

func (s *runtimeSink) Resolve(model.ObjectRef, string) {}

func (s *runtimeSink) ResolveObserved(*model.Observation) {}

func (s *runtimeSink) RemovePodWithUID(string, string, string) {
	s.removed++
}

func (s *runtimeSink) ResolveHealthyPodContainers(
	string, string, map[string]bool,
) {
	s.recovered++
}

func (s *runtimeSink) ClearBaselineForPod(
	string, string, model.ObjectRef,
) {
	s.cleared++
}

func newPodLister(pods ...*corev1.Pod) corev1listers.PodLister {
	indexer := cache.NewIndexer(
		cache.MetaNamespaceKeyFunc,
		cache.Indexers{},
	)
	for _, pod := range pods {
		if err := indexer.Add(pod); err != nil {
			panic(err)
		}
	}
	return corev1listers.NewPodLister(indexer)
}

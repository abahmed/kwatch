package metricsapi

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

func TestPodsFromCacheAppliesNamespaceFilter(t *testing.T) {
	index := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	for _, pod := range []*corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "apps"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "system", Namespace: "system"}},
	} {
		if err := index.Add(pod); err != nil {
			t.Fatal(err)
		}
	}
	monitor := &Monitor{
		podLister: corev1listers.NewPodLister(index),
		allowed:   func(namespace string) bool { return namespace == "apps" },
	}
	got, err := monitor.podsFromCache("")
	if err != nil || len(got) != 1 || got["apps/api"] == nil {
		t.Fatalf("cached pods = %v, %v", got, err)
	}
}

func TestProcessPodReportsUsageAndResolvesMissingContainers(t *testing.T) {
	sink := &metricsSink{}
	monitor := &Monitor{
		incidentSink: sink,
		cfg: config.RuntimeMetricsMonitor{
			MemoryWarningPercent: 50, MemoryCriticalPercent: 80,
		},
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Namespace: "apps", Name: "api",
	}, Spec: corev1.PodSpec{Containers: []corev1.Container{
		{Name: "app",
			Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("100Mi"),
			}}},
		{Name: "sidecar",
			Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("100Mi"),
			}}},
	}}}
	metrics := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"namespace": "apps", "name": "api"},
		"containers": []interface{}{map[string]interface{}{
			"name": "app", "usage": map[string]interface{}{
				"memory": "90Mi",
			},
		}},
	}}
	monitor.processPod(pod, metrics)
	if len(sink.processed) != 1 || sink.processed[0].Reason !=
		constant.ReasonContainerMemoryHigh {
		t.Fatalf("processed observations = %+v", sink.processed)
	}
	if len(sink.resolved) != 2 || sink.resolved[0].Container != "sidecar" {
		t.Fatalf("resolved observations = %+v", sink.resolved)
	}
}

func TestConfigureSourcesCopiesScopeAndRejectsMutation(t *testing.T) {
	monitor := &Monitor{}
	namespaces := []string{"apps"}
	if err := monitor.ConfigureSources(Sources{
		Namespaces: namespaces, WatchAll: false,
	}); err != nil {
		t.Fatal(err)
	}
	namespaces[0] = "changed"
	if monitor.namespaces[0] != "apps" {
		t.Fatal("source configuration retained caller slice")
	}
	if err := monitor.ConfigureSources(Sources{}); err == nil {
		t.Fatal("second ConfigureSources() succeeded")
	}
	monitor = &Monitor{started: true}
	if err := monitor.ConfigureSources(Sources{}); err == nil {
		t.Fatal("ConfigureSources() succeeded after start")
	}
}

func TestPodOwnerUsesResolverAndSafeFallbacks(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Namespace: "apps", Name: "api",
	}}
	monitor := &Monitor{owners: observe.OwnerFunc(func(
		*corev1.Pod,
	) model.ObjectRef {
		return model.ObjectRef{Kind: "Deployment", Name: "api"}
	})}
	if got := monitor.podOwner(pod); got.Kind != "Deployment" {
		t.Fatalf("resolved owner = %+v", got)
	}
	monitor.owners = observe.OwnerFunc(func(*corev1.Pod) model.ObjectRef {
		return model.ObjectRef{}
	})
	if got := monitor.podOwner(pod); got.Kind != "Pod" {
		t.Fatalf("ownerless fallback = %+v", got)
	}
	pod.OwnerReferences = []metav1.OwnerReference{{
		Kind: "ReplicaSet",
		Name: "api",
	}}
	if got := monitor.podOwner(pod); got.Name != "apps/api" || got.Kind != "" {
		t.Fatalf("unresolved owner fallback = %+v", got)
	}
}

package kubeclient

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

type stubObject struct{ name string }

func panickingTransform(obj any) (any, error) { panic("poison") }

func TestSafeTransformReplacesTheObjectOfAPanic(t *testing.T) {
	transform := SafeTransform("test", "widgets", panickingTransform,
		func(obj any) any { return stubObject{name: "stub"} })

	got, err := transform(&corev1.Pod{})

	require.NoError(t, err)
	require.Equal(t, stubObject{name: "stub"}, got)
}

func TestSafeTransformPassesResultsThrough(t *testing.T) {
	want := &corev1.Pod{}
	transform := SafeTransform("test", "widgets",
		func(obj any) (any, error) { return obj, nil },
		func(any) any { t.Fatal("stub used without a panic"); return nil })

	got, err := transform(want)

	require.NoError(t, err)
	require.Same(t, want, got)
}

func TestSafeTransformRecordsPanicsPerResource(t *testing.T) {
	count := func(resource string) int64 {
		for _, p := range HandlerPanics() {
			if p.Component == "transform-test" && p.Resource == resource {
				return p.Count
			}
		}
		return 0
	}
	before := count("widgets")
	transform := SafeTransform("transform-test", "widgets",
		panickingTransform, func(obj any) any { return obj })

	_, _ = transform(&corev1.Pod{})
	_, _ = transform(&corev1.Pod{})

	require.Equal(t, before+2, count("widgets"))
}

func TestSafeTransformNamesAnUnnamedResourceAfterTheObjectType(t *testing.T) {
	transform := SafeTransform("transform-test", "", panickingTransform,
		func(obj any) any { return obj })

	_, _ = transform(&corev1.Pod{})

	found := false
	for _, p := range HandlerPanics() {
		if p.Component == "transform-test" && p.Resource == "*v1.Pod" {
			found = true
		}
	}
	require.True(t, found, "the ledger should name the object's type")
}

// The reflector runs transforms on its own goroutine: an unguarded panic
// ends the process. With the guard the informer keeps delivering.
func TestSafeTransformKeepsAnInformerRunning(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Name: "poison", Namespace: "ns"}})
	factory := informers.NewSharedInformerFactoryWithOptions(client, 0)
	informer := factory.Core().V1().Pods().Informer()
	stub := func(obj any) any {
		pod := obj.(*corev1.Pod)
		return &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{
			Name: pod.Name, Namespace: pod.Namespace}}
	}
	require.NoError(t, informer.SetTransform(SafeTransform(
		"test", "pods", panickingTransform, stub)))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go informer.Run(ctx.Done())

	require.True(t, cache.WaitForCacheSync(ctx.Done(), informer.HasSynced))
	items := informer.GetStore().List()
	require.Len(t, items, 1)
	stubbed, ok := items[0].(*metav1.PartialObjectMetadata)
	require.True(t, ok, "the cache holds the stub, got %T", items[0])
	require.Equal(t, "poison", stubbed.Name)
}

package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	kwcontext "github.com/abahmed/kwatch/internal/context"
)

func TestRecordChangeUsesInjectedClock(t *testing.T) {
	const want = "2026-08-31T12:34:56Z"
	now, err := time.Parse(time.RFC3339, want)
	require.NoError(t, err)
	controller := &Controller{tracker: kwcontext.NewChangeTracker(10), now: func() time.Time { return now }}
	obj := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "prod"}}

	controller.recordChange(kwcontext.ChangeUpdate, "service", obj)
	changes := controller.tracker.Snapshot()
	require.Len(t, changes, 1)
	require.Equal(t, now, changes[0].Timestamp)
}

func TestServiceDependentsFallbackWhenGraphHasNoEdge(t *testing.T) {
	client := fake.NewSimpleClientset()
	factory := informers.NewSharedInformerFactory(client, 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	factory.Start(ctx.Done())
	factory.WaitForCacheSync(ctx.Done())
	require.NoError(t, factory.Networking().V1().Ingresses().Informer().GetStore().Add(&networkingIngress))

	controller := &Controller{
		graph:         kwcontext.NewResourceGraph(),
		ingress:       newResourcePipeline("ingress", "ingresses-test"),
		mwc:           newResourcePipeline("mwc", "mwc-test"),
		vwc:           newResourcePipeline("vwc", "vwc-test"),
		ingressLister: factory.Networking().V1().Ingresses().Lister(),
	}
	controller.ingress.startWorkers = true
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "prod"}}

	controller.enqueueServiceDependents(service)
	require.Equal(t, 1, controller.ingress.queue.Len())
}

var networkingIngress = networkingv1.Ingress{
	ObjectMeta: metav1.ObjectMeta{Name: "api-ingress", Namespace: "prod"},
}

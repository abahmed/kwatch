package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
)

func TestProcessIngressObjectAlreadyResolved(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "my-svc", Namespace: "default"},
	}
	f.Core().V1().Services().Informer().GetIndexer().Add(svc)
	h.listers.Service = f.Core().V1().Services().Lister()

	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "test-ing", Namespace: "default"},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path: "/",
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "my-svc",
											Port: networkingv1.ServiceBackendPort{
												Number: 80,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	assert.NoError(t, h.ProcessIngressObject(ing, false))
	assert.Equal(
		t,
		0,
		e.ActiveCount(),
		"ingress with valid backend should not create incident",
	)
}

func TestProcessIngressObjectDeleted(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	assert.NoError(t, h.ProcessIngress("default/test-ing", true))
}

func TestProcessIngressObjectNil(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	assert.NoError(t, h.ProcessIngressObject(nil, false))
}

func TestProcessIngressInvalidKey(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	assert.Error(t, h.ProcessIngress("a/b/c", false))
}

func TestProcessIngressObjectDeletedPath(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "test-ing", Namespace: "default"},
	}
	assert.NoError(t, h.ProcessIngressObject(ing, true))
	assert.Equal(t, 0, e.ActiveCount())
}

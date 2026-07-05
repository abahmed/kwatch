package handler

import (
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

func TestProcessIngressCreatesIncident(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.SetServiceLister(f.Core().V1().Services().Lister())
	h.SetIngressLister(f.Networking().V1().Ingresses().Lister())

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
											Name: "missing-svc",
											Port: networkingv1.ServiceBackendPort{Number: 80},
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
	f.Networking().V1().Ingresses().Informer().GetIndexer().Add(ing)

	assert.NoError(t, h.ProcessIngress("default/test-ing", false))

	snap := e.Snapshot()
	var found bool
	for _, v := range snap {
		if v.Reason == "IngressBackendNotFound" && v.State == model.StateActive {
			found = true
		}
	}
	assert.True(t, found, "key-based ProcessIngress should create incident")
}

func TestProcessIngressKeyDeleted(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	assert.NoError(t, h.ProcessIngress("default/test-ing", true))
}

func TestProcessIngressKeyNotFound(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.SetIngressLister(f.Networking().V1().Ingresses().Lister())

	assert.NoError(t, h.ProcessIngress("default/missing", false))
	assert.Equal(t, 0, e.ActiveCount())
}

func TestDetectIngressIssueNoRules(t *testing.T) {
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "test-ing", Namespace: "default"},
	}
	hasService := func(ns, name string) bool { return false }
	sigs := DetectIngressIssue(ing, hasService)
	assert.Empty(t, sigs)
}

func TestDetectIngressIssueNoHTTP(t *testing.T) {
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "test-ing", Namespace: "default"},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: nil,
					},
				},
			},
		},
	}
	hasService := func(ns, name string) bool { return false }
	sigs := DetectIngressIssue(ing, hasService)
	assert.Empty(t, sigs)
}

func TestDetectIngressIssueNoServiceBackend(t *testing.T) {
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
										Service:  nil,
										Resource: nil,
									},
								},
							},
						},
					},
				},
			},
		},
	}
	hasService := func(ns, name string) bool { return false }
	sigs := DetectIngressIssue(ing, hasService)
	assert.Empty(t, sigs, "paths without service backend should not produce a signal")
}

func TestDetectIngressIssueServiceExists(t *testing.T) {
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
											Port: networkingv1.ServiceBackendPort{Number: 80},
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
	hasService := func(ns, name string) bool {
		return ns == "default" && name == "my-svc"
	}
	sigs := DetectIngressIssue(ing, hasService)
	assert.Empty(t, sigs)
}

func TestDetectIngressIssueServiceMissing(t *testing.T) {
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
											Name: "missing-svc",
											Port: networkingv1.ServiceBackendPort{Number: 80},
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
	hasService := func(ns, name string) bool { return false }
	sigs := DetectIngressIssue(ing, hasService)
	assert.Len(t, sigs, 1)
	assert.Equal(t, "IngressBackendNotFound", sigs[0].Reason)
	assert.Equal(t, "ingress", sigs[0].Resource)
	assert.Equal(t, "default/test-ing", sigs[0].Owner)
}

func TestDetectIngressIssueDefaultBackendMissing(t *testing.T) {
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "test-ing", Namespace: "default"},
		Spec: networkingv1.IngressSpec{
			DefaultBackend: &networkingv1.IngressBackend{
				Service: &networkingv1.IngressServiceBackend{
					Name: "default-backend-svc",
					Port: networkingv1.ServiceBackendPort{Number: 80},
				},
			},
		},
	}
	hasService := func(ns, name string) bool { return false }
	sigs := DetectIngressIssue(ing, hasService)
	assert.Len(t, sigs, 1)
	assert.Equal(t, "IngressBackendNotFound", sigs[0].Reason)
	assert.Contains(t, sigs[0].Hint, "default-backend")
}

func TestDetectIngressIssueMultipleMissing(t *testing.T) {
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "test-ing", Namespace: "default"},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: "a.example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path: "/",
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "svc-a",
										},
									},
								},
							},
						},
					},
				},
				{
					Host: "b.example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path: "/",
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "svc-b",
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
	hasService := func(ns, name string) bool { return false }
	sigs := DetectIngressIssue(ing, hasService)
	assert.Len(t, sigs, 2, "both missing backends should produce signals")
}

func TestProcessIngressObjectCreatesAndResolves(t *testing.T) {
	var mu sync.Mutex
	var resolves int

	e := correlation.NewEngine(correlation.Config{
		Window: 10 * time.Minute,
		LifecycleHook: func(inc *model.Incident, action model.IncidentAction) {
			mu.Lock()
			defer mu.Unlock()
			if action == model.ActionResolved {
				resolves++
			}
		},
	})

	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	// Set up service lister with NO services so lookups fail
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.SetServiceLister(f.Core().V1().Services().Lister())

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
											Name: "missing-svc",
											Port: networkingv1.ServiceBackendPort{Number: 80},
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

	// No services in lister — should create incident
	assert.NoError(t, h.ProcessIngressObject(ing, false))
	assert.Equal(t, 1, e.ActiveCount())

	snap := e.Snapshot()
	var found bool
	for _, v := range snap {
		if v.Reason == "IngressBackendNotFound" {
			found = true
		}
	}
	assert.True(t, found, "IngressBackendNotFound incident should exist")

	// Add the missing service and re-process — should resolve
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "missing-svc", Namespace: "default"},
	}
	f.Core().V1().Services().Informer().GetIndexer().Add(svc)

	assert.NoError(t, h.ProcessIngressObject(ing, false))

	mu.Lock()
	r := resolves
	mu.Unlock()
	assert.Equal(t, 1, r, "IngressBackendNotFound should resolve when service exists")
	assert.Equal(t, 0, e.ActiveCount())
}

func TestProcessIngressObjectAlreadyResolved(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "my-svc", Namespace: "default"},
	}
	f.Core().V1().Services().Informer().GetIndexer().Add(svc)
	h.SetServiceLister(f.Core().V1().Services().Lister())

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
											Port: networkingv1.ServiceBackendPort{Number: 80},
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
	assert.Equal(t, 0, e.ActiveCount(), "ingress with valid backend should not create incident")
}

func TestProcessIngressObjectDeleted(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	assert.NoError(t, h.ProcessIngress("default/test-ing", true))
}

func TestProcessIngressObjectNil(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	assert.NoError(t, h.ProcessIngressObject(nil, false))
}

func TestProcessIngressInvalidKey(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	assert.Error(t, h.ProcessIngress("a/b/c", false))
}

func TestProcessIngressObjectDeletedPath(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "test-ing", Namespace: "default"},
	}
	assert.NoError(t, h.ProcessIngressObject(ing, true))
	assert.Equal(t, 0, e.ActiveCount())
}

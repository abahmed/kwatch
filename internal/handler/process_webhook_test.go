package handler

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/model"
)

func TestDetectMutatingWebhookNoServiceRef(t *testing.T) {
	mwc := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mwc"},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{
				Name: "hook.example.com",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: nil,
				},
			},
		},
	}
	hasService := func(ns, name string) bool { return false }
	sigs := DetectMutatingWebhookIssue(mwc, hasService)
	assert.Empty(t, sigs, "webhook without service ref should not produce a signal")
}

func TestDetectMutatingWebhookServiceExists(t *testing.T) {
	mwc := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mwc"},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{
				Name: "hook.example.com",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{
						Namespace: "default",
						Name:      "my-webhook-svc",
					},
				},
			},
		},
	}
	hasService := func(ns, name string) bool {
		return ns == "default" && name == "my-webhook-svc"
	}
	sigs := DetectMutatingWebhookIssue(mwc, hasService)
	assert.Empty(t, sigs, "webhook with existing service should not produce a signal")
}

func TestDetectMutatingWebhookServiceMissing(t *testing.T) {
	mwc := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mwc"},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{
				Name: "hook.example.com",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{
						Namespace: "default",
						Name:      "missing-svc",
					},
				},
			},
		},
	}
	hasService := func(ns, name string) bool { return false }
	sigs := DetectMutatingWebhookIssue(mwc, hasService)
	assert.Len(t, sigs, 1)
	assert.Equal(t, "WebhookBackendNotFound", sigs[0].Reason)
	assert.Equal(t, "mutatingwebhookconfiguration", sigs[0].Resource)
	assert.Equal(t, "test-mwc", sigs[0].Owner)
}

func TestDetectMutatingWebhookMultipleHooks(t *testing.T) {
	mwc := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mwc"},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{
				Name: "hook1.example.com",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{
						Namespace: "default", Name: "svc1",
					},
				},
			},
			{
				Name: "hook2.example.com",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{
						Namespace: "default", Name: "svc2",
					},
				},
			},
			{
				Name: "hook3.example.com",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: nil,
				},
			},
		},
	}
	hasService := func(ns, name string) bool {
		return name == "svc1"
	}
	sigs := DetectMutatingWebhookIssue(mwc, hasService)
	assert.Len(t, sigs, 1, "only the missing service should produce a signal")
	assert.Contains(t, sigs[0].Hint, "svc2")
}

func TestDetectValidatingWebhookNoServiceRef(t *testing.T) {
	vwc := &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "test-vwc"},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{
			{
				Name: "hook.example.com",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: nil,
				},
			},
		},
	}
	hasService := func(ns, name string) bool { return false }
	sigs := DetectValidatingWebhookIssue(vwc, hasService)
	assert.Empty(t, sigs)
}

func TestDetectValidatingWebhookServiceExists(t *testing.T) {
	vwc := &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "test-vwc"},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{
			{
				Name: "hook.example.com",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{
						Namespace: "default",
						Name:      "my-webhook-svc",
					},
				},
			},
		},
	}
	hasService := func(ns, name string) bool {
		return ns == "default" && name == "my-webhook-svc"
	}
	sigs := DetectValidatingWebhookIssue(vwc, hasService)
	assert.Empty(t, sigs)
}

func TestDetectValidatingWebhookServiceMissing(t *testing.T) {
	vwc := &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "test-vwc"},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{
			{
				Name: "hook.example.com",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{
						Namespace: "default",
						Name:      "missing-svc",
					},
				},
			},
		},
	}
	hasService := func(ns, name string) bool { return false }
	sigs := DetectValidatingWebhookIssue(vwc, hasService)
	assert.Len(t, sigs, 1)
	assert.Equal(t, "WebhookBackendNotFound", sigs[0].Reason)
	assert.Equal(t, "validatingwebhookconfiguration", sigs[0].Resource)
}

func TestProcessMutatingWebhookObjectCreatesAndResolves(t *testing.T) {
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

	mwc := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mwc"},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{
				Name: "hook.example.com",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{
						Namespace: "default",
						Name:      "missing-svc",
					},
				},
			},
		},
	}

	// No services in lister — should create incident
	assert.NoError(t, h.ProcessMutatingWebhookConfigurationObject(mwc, false))
	assert.Equal(t, 1, e.ActiveCount())

	snap := e.Snapshot()
	var found bool
	for _, v := range snap {
		if v.Reason == "WebhookBackendNotFound" {
			found = true
		}
	}
	assert.True(t, found, "WebhookBackendNotFound incident should exist")

	// Add the missing service to the lister and re-process — should resolve
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "missing-svc", Namespace: "default"},
	}
	f.Core().V1().Services().Informer().GetIndexer().Add(svc)

	assert.NoError(t, h.ProcessMutatingWebhookConfigurationObject(mwc, false))

	mu.Lock()
	r := resolves
	mu.Unlock()
	assert.Equal(t, 1, r, "WebhookBackendNotFound should resolve when service exists")
	assert.Equal(t, 0, e.ActiveCount())
}

func TestProcessValidatingWebhookObjectCreatesAndResolves(t *testing.T) {
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

	vwc := &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "test-vwc"},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{
			{
				Name: "hook.example.com",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{
						Namespace: "default",
						Name:      "missing-svc",
					},
				},
			},
		},
	}

	assert.NoError(t, h.ProcessValidatingWebhookConfigurationObject(vwc, false))
	assert.Equal(t, 1, e.ActiveCount())

	snap := e.Snapshot()
	var found bool
	for _, v := range snap {
		if v.Reason == "WebhookBackendNotFound" {
			found = true
		}
	}
	assert.True(t, found)

	// Add the missing service to the lister and re-process — should resolve
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "missing-svc", Namespace: "default"},
	}
	f.Core().V1().Services().Informer().GetIndexer().Add(svc)

	assert.NoError(t, h.ProcessValidatingWebhookConfigurationObject(vwc, false))

	mu.Lock()
	r := resolves
	mu.Unlock()
	assert.Equal(t, 1, r, "WebhookBackendNotFound should resolve when service exists")
	assert.Equal(t, 0, e.ActiveCount())
}

func TestProcessMutatingWebhookObjectDeleted(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	assert.NoError(t, h.ProcessMutatingWebhookConfiguration("test-mwc", true))
}

func TestProcessValidatingWebhookObjectDeleted(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	assert.NoError(t, h.ProcessValidatingWebhookConfiguration("test-vwc", true))
}

func TestProcessMutatingWebhookObjectNil(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	assert.NoError(t, h.ProcessMutatingWebhookConfigurationObject(nil, false))
}

func TestProcessValidatingWebhookObjectNil(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	assert.NoError(t, h.ProcessValidatingWebhookConfigurationObject(nil, false))
}

func TestProcessMutatingWebhookObjectNoServiceRefNoIncident(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	mwc := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mwc"},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{
				Name: "hook.example.com",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: nil,
				},
			},
		},
	}
	assert.NoError(t, h.ProcessMutatingWebhookConfigurationObject(mwc, false))
	assert.Equal(t, 0, e.ActiveCount())
}

func TestProcessMutatingWebhookConfigurationKeyNotFound(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.SetMwCLister(f.Admissionregistration().V1().MutatingWebhookConfigurations().Lister())

	assert.NoError(t, h.ProcessMutatingWebhookConfiguration("missing-mwc", false))
	assert.Equal(t, 0, e.ActiveCount())
}

func TestProcessMutatingWebhookConfigurationKeyValid(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.SetMwCLister(f.Admissionregistration().V1().MutatingWebhookConfigurations().Lister())
	h.SetServiceLister(f.Core().V1().Services().Lister())

	mwc := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mwc"},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{
				Name: "hook.example.com",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{
						Namespace: "default",
						Name:      "missing-svc",
					},
				},
			},
		},
	}
	f.Admissionregistration().V1().MutatingWebhookConfigurations().Informer().GetIndexer().Add(mwc)

	assert.NoError(t, h.ProcessMutatingWebhookConfiguration("test-mwc", false))

	snap := e.Snapshot()
	var found bool
	for _, v := range snap {
		if v.Reason == "WebhookBackendNotFound" {
			found = true
		}
	}
	assert.True(t, found, "key-based ProcessMutatingWebhookConfiguration should create incident")
}

func TestProcessValidatingWebhookConfigurationKeyNotFound(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.SetVwCLister(f.Admissionregistration().V1().ValidatingWebhookConfigurations().Lister())

	assert.NoError(t, h.ProcessValidatingWebhookConfiguration("missing-vwc", false))
	assert.Equal(t, 0, e.ActiveCount())
}

func TestProcessValidatingWebhookConfigurationKeyValid(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.SetVwCLister(f.Admissionregistration().V1().ValidatingWebhookConfigurations().Lister())
	h.SetServiceLister(f.Core().V1().Services().Lister())

	vwc := &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "test-vwc"},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{
			{
				Name: "hook.example.com",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{
						Namespace: "default",
						Name:      "missing-svc",
					},
				},
			},
		},
	}
	f.Admissionregistration().V1().ValidatingWebhookConfigurations().Informer().GetIndexer().Add(vwc)

	assert.NoError(t, h.ProcessValidatingWebhookConfiguration("test-vwc", false))

	snap := e.Snapshot()
	var found bool
	for _, v := range snap {
		if v.Reason == "WebhookBackendNotFound" {
			found = true
		}
	}
	assert.True(t, found, "key-based ProcessValidatingWebhookConfiguration should create incident")
}

func TestProcessMutatingWebhookConfigurationObjectDeleted(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	mwc := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mwc"},
	}
	assert.NoError(t, h.ProcessMutatingWebhookConfigurationObject(mwc, true))
	assert.Equal(t, 0, e.ActiveCount())
}

func TestProcessValidatingWebhookConfigurationObjectDeleted(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	vwc := &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "test-vwc"},
	}
	assert.NoError(t, h.ProcessValidatingWebhookConfigurationObject(vwc, true))
	assert.Equal(t, 0, e.ActiveCount())
}

func TestProcessMutatingWebhookConfigurationObjectNoServiceLister(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	mwc := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mwc"},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{
				Name: "hook.example.com",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{
						Namespace: "default",
						Name:      "some-svc",
					},
				},
			},
		},
	}
	assert.NoError(t, h.ProcessMutatingWebhookConfigurationObject(mwc, false))
	assert.Equal(t, 0, e.ActiveCount(), "no service lister assumes ok, no incident")
}

func TestProcessValidatingWebhookConfigurationObjectNoServiceLister(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	vwc := &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "test-vwc"},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{
			{
				Name: "hook.example.com",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{
						Namespace: "default",
						Name:      "some-svc",
					},
				},
			},
		},
	}
	assert.NoError(t, h.ProcessValidatingWebhookConfigurationObject(vwc, false))
	assert.Equal(t, 0, e.ActiveCount(), "no service lister assumes ok, no incident")
}

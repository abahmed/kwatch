package handler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	admissionv1informers "k8s.io/client-go/informers/admissionregistration/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
)

func TestProcessValidatingWebhookObjectDeleted(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	assert.NoError(t, h.ProcessValidatingWebhookConfiguration("test-vwc", true))
}

func TestProcessMutatingWebhookObjectNil(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	assert.NoError(t, h.ProcessMutatingWebhookConfigurationObject(nil, false))
}

func TestProcessValidatingWebhookObjectNil(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	assert.NoError(t, h.ProcessValidatingWebhookConfigurationObject(nil, false))
}

func TestProcessMutatingWebhookObjectNoServiceRefNoIncident(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

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
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.listers.MWC = admv1(f).MutatingWebhookConfigurations().Lister()

	assert.NoError(
		t,
		h.ProcessMutatingWebhookConfiguration("missing-mwc", false),
	)
	assert.Equal(t, 0, e.ActiveCount())
}

func TestProcessMutatingWebhookConfigurationKeyValid(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.listers.MWC = admv1(f).MutatingWebhookConfigurations().Lister()
	h.listers.Service = f.Core().V1().Services().Lister()

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
	admv1(f).MutatingWebhookConfigurations().Informer().GetIndexer().Add(
		mwc,
	)

	assert.NoError(t, h.ProcessMutatingWebhookConfiguration("test-mwc", false))

	snap := e.Snapshot()
	var found bool
	for _, v := range snap {
		if v.Reason == "WebhookBackendNotFound" {
			found = true
		}
	}
	assert.True(
		t,
		found,
		"key-based ProcessMutatingWebhookConfiguration should create incident",
	)
}

func TestProcessValidatingWebhookConfigurationKeyNotFound(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.listers.VWC = admv1(f).ValidatingWebhookConfigurations().Lister()

	assert.NoError(
		t,
		h.ProcessValidatingWebhookConfiguration("missing-vwc", false),
	)
	assert.Equal(t, 0, e.ActiveCount())
}

func TestProcessValidatingWebhookConfigurationKeyValid(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.listers.VWC = admv1(f).ValidatingWebhookConfigurations().Lister()
	h.listers.Service = f.Core().V1().Services().Lister()

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
	admv1(f).ValidatingWebhookConfigurations().Informer().GetIndexer().Add(
		vwc,
	)

	assert.NoError(
		t,
		h.ProcessValidatingWebhookConfiguration("test-vwc", false),
	)

	snap := e.Snapshot()
	var found bool
	for _, v := range snap {
		if v.Reason == "WebhookBackendNotFound" {
			found = true
		}
	}
	assert.True(
		t,
		found,
		"key-based ProcessValidatingWebhookConfiguration should create "+
			"incident",
	)
}

func TestProcessMutatingWebhookConfigurationObjectDeleted(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

	mwc := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mwc"},
	}
	assert.NoError(t, h.ProcessMutatingWebhookConfigurationObject(mwc, true))
	assert.Equal(t, 0, e.ActiveCount())
}

func TestProcessValidatingWebhookConfigurationObjectDeleted(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

	vwc := &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "test-vwc"},
	}
	assert.NoError(t, h.ProcessValidatingWebhookConfigurationObject(vwc, true))
	assert.Equal(t, 0, e.ActiveCount())
}

func TestProcessMutatingWebhookConfigurationObjectNoServiceLister(
	t *testing.T,
) {
	e := testCorrelator()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

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
	assert.Equal(
		t,
		0,
		e.ActiveCount(),
		"no service lister assumes ok, no incident",
	)
}

func TestProcessValidatingWebhookConfigurationObjectNoServiceLister(
	t *testing.T,
) {
	e := testCorrelator()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

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
	assert.Equal(
		t,
		0,
		e.ActiveCount(),
		"no service lister assumes ok, no incident",
	)
}

// admv1 shortens the admissionregistration/v1 informer group in tests.
func admv1(f informers.SharedInformerFactory) admissionv1informers.Interface {
	return f.Admissionregistration().V1()
}

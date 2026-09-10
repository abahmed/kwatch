package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
)

func TestProcessIngressUnknownServiceStateKeepsIncident(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{})
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.listers.Service = f.Core().V1().Services().Lister()
	ing := testIngressWithService("missing-service")

	assert.NoError(t, h.ProcessIngressObject(ing, false))
	assert.Equal(t, 1, e.ActiveCount())

	h.listers.Service = &errorServiceLister{
		ServiceLister: f.Core().V1().Services().Lister(),
	}
	err := h.ProcessIngressObject(ing, false)

	assert.Error(t, err)
	assert.Equal(t, 1, e.ActiveCount())
}

func TestProcessMutatingWebhookUnknownServiceStateKeepsIncident(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{})
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.listers.Service = f.Core().V1().Services().Lister()
	mwc := testMutatingWebhookWithService("missing-service")

	assert.NoError(t, h.ProcessMutatingWebhookConfigurationObject(mwc, false))
	assert.Equal(t, 1, e.ActiveCount())

	h.listers.Service = &errorServiceLister{
		ServiceLister: f.Core().V1().Services().Lister(),
	}
	err := h.ProcessMutatingWebhookConfigurationObject(mwc, false)

	assert.Error(t, err)
	assert.Equal(t, 1, e.ActiveCount())
}

func TestProcessValidatingWebhookUnknownServiceStateKeepsIncident(
	t *testing.T,
) {
	e := correlation.NewEngine(correlation.Config{})
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.listers.Service = f.Core().V1().Services().Lister()
	vwc := testValidatingWebhookWithService("missing-service")

	assert.NoError(t, h.ProcessValidatingWebhookConfigurationObject(vwc, false))
	assert.Equal(t, 1, e.ActiveCount())

	h.listers.Service = &errorServiceLister{
		ServiceLister: f.Core().V1().Services().Lister(),
	}
	err := h.ProcessValidatingWebhookConfigurationObject(vwc, false)

	assert.Error(t, err)
	assert.Equal(t, 1, e.ActiveCount())
}

func TestProcessWebhookUnknownEndpointStateKeepsIncident(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{})
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.listers.EndpointSlice = f.Discovery().V1().EndpointSlices().Lister()
	mwc := testMutatingWebhookWithService("webhook-service")

	assert.NoError(t, h.ProcessMutatingWebhookConfigurationObject(mwc, false))
	assert.Equal(t, 1, e.ActiveCount())

	h.listers.EndpointSlice = &errorEndpointSliceLister{
		EndpointSliceLister: f.Discovery().V1().EndpointSlices().Lister(),
	}
	err := h.ProcessMutatingWebhookConfigurationObject(mwc, false)

	assert.Error(t, err)
	assert.Equal(t, 1, e.ActiveCount())
}

func TestDetectIngressIssueWithLookupReturnsError(t *testing.T) {
	ing := testIngressWithService("backend")
	wantErr := assert.AnError

	findings, err := DetectIngressIssueWithLookup(
		ing,
		func(string, string) (bool, error) {
			return false, wantErr
		},
	)

	assert.ErrorIs(t, err, wantErr)
	assert.Nil(t, findings)
}

func TestDetectWebhookEndpointIssuesWithErrorReturnsError(t *testing.T) {
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	refs := []*admissionregistrationv1.ServiceReference{
		{Namespace: "default", Name: "backend"},
	}
	lister := &errorEndpointSliceLister{
		EndpointSliceLister: f.Discovery().V1().EndpointSlices().Lister(),
	}

	findings, err := DetectWebhookEndpointIssuesWithError(
		lister,
		"mwc",
		"",
		nil,
		refs,
	)

	assert.Error(t, err)
	assert.Nil(t, findings)
}

func testIngressWithService(name string) *networkingv1.Ingress {
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "test-ingress", Namespace: "default"},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: name,
								},
							},
						}},
					},
				},
			}},
		},
	}
}

func testMutatingWebhookWithService(
	name string,
) *admissionregistrationv1.MutatingWebhookConfiguration {
	return &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mwc"},
		Webhooks: []admissionregistrationv1.MutatingWebhook{{
			ClientConfig: admissionregistrationv1.WebhookClientConfig{
				Service: &admissionregistrationv1.ServiceReference{
					Namespace: "default",
					Name:      name,
				},
			},
		}},
	}
}

func testValidatingWebhookWithService(
	name string,
) *admissionregistrationv1.ValidatingWebhookConfiguration {
	return &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "test-vwc"},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{{
			ClientConfig: admissionregistrationv1.WebhookClientConfig{
				Service: &admissionregistrationv1.ServiceReference{
					Namespace: "default",
					Name:      name,
				},
			},
		}},
	}
}

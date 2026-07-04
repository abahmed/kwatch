package handler

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/api/errors"
)

// serviceRef returns "namespace/name" for a webhook's ServiceReference, or "".
func serviceRef(svc *admissionregistrationv1.ServiceReference) string {
	if svc == nil {
		return ""
	}
	return svc.Namespace + "/" + svc.Name
}

// DetectMutatingWebhookIssue checks a MutatingWebhookConfiguration for webhooks
// whose service backend doesn't exist.
func DetectMutatingWebhookIssue(mwc *admissionregistrationv1.MutatingWebhookConfiguration, hasService func(ns, name string) bool) []*event.Signal {
	var sigs []*event.Signal
	for _, w := range mwc.Webhooks {
		ref := serviceRef(w.ClientConfig.Service)
		if ref == "" {
			continue
		}
		ns, name := w.ClientConfig.Service.Namespace, w.ClientConfig.Service.Name
		if !hasService(ns, name) {
			sigs = append(sigs, &event.Signal{
				Resource:  "mutatingwebhookconfiguration",
				Namespace: mwc.Namespace,
				Reason:    "WebhookBackendNotFound",
				Owner:     mwc.Name,
				PodName:   mwc.Name,
				Labels:    mwc.Labels,
				Hint:      fmt.Sprintf("mutating webhook %q: service %s/%s does not exist", mwc.Name, ns, name),
			})
		}
	}
	return sigs
}

// DetectValidatingWebhookIssue checks a ValidatingWebhookConfiguration for webhooks
// whose service backend doesn't exist.
func DetectValidatingWebhookIssue(vwc *admissionregistrationv1.ValidatingWebhookConfiguration, hasService func(ns, name string) bool) []*event.Signal {
	var sigs []*event.Signal
	for _, w := range vwc.Webhooks {
		ref := serviceRef(w.ClientConfig.Service)
		if ref == "" {
			continue
		}
		ns, name := w.ClientConfig.Service.Namespace, w.ClientConfig.Service.Name
		if !hasService(ns, name) {
			sigs = append(sigs, &event.Signal{
				Resource:  "validatingwebhookconfiguration",
				Namespace: vwc.Namespace,
				Reason:    "WebhookBackendNotFound",
				Owner:     vwc.Name,
				PodName:   vwc.Name,
				Labels:    vwc.Labels,
				Hint:      fmt.Sprintf("validating webhook %q: service %s/%s does not exist", vwc.Name, ns, name),
			})
		}
	}
	return sigs
}

func (h *handler) ProcessMutatingWebhookConfiguration(key string, deleted bool) error {
	if deleted {
		h.correlator.ResolveByResource("mutatingwebhookconfiguration", key)
		return nil
	}
	mwc, err := h.mwcLister.Get(key)
	if err != nil {
		if errors.IsNotFound(err) {
			h.correlator.ResolveByResource("mutatingwebhookconfiguration", key)
			return nil
		}
		return fmt.Errorf("failed to get mutatingwebhookconfiguration %s from cache: %w", key, err)
	}
	return h.ProcessMutatingWebhookConfigurationObject(mwc, false)
}

func (h *handler) ProcessMutatingWebhookConfigurationObject(mwc *admissionregistrationv1.MutatingWebhookConfiguration, deleted bool) error {
	if mwc == nil {
		return nil
	}
	if deleted {
		h.correlator.ResolveByResource("mutatingwebhookconfiguration", mwc.Name)
		return nil
	}

	hasService := func(ns, name string) bool {
		if h.serviceLister == nil {
			return true // can't check, assume ok
		}
		_, err := h.serviceLister.Services(ns).Get(name)
		return err == nil
	}

	sigs := DetectMutatingWebhookIssue(mwc, hasService)
	for _, sig := range sigs {
		h.signalEvent(sig)
	}
	if len(sigs) == 0 {
		h.correlator.MarkResolved(correlation.BuildKey("", mwc.Name, "WebhookBackendNotFound", ""))
	}
	return nil
}

func (h *handler) ProcessValidatingWebhookConfiguration(key string, deleted bool) error {
	if deleted {
		h.correlator.ResolveByResource("validatingwebhookconfiguration", key)
		return nil
	}
	vwc, err := h.vwcLister.Get(key)
	if err != nil {
		if errors.IsNotFound(err) {
			h.correlator.ResolveByResource("validatingwebhookconfiguration", key)
			return nil
		}
		return fmt.Errorf("failed to get validatingwebhookconfiguration %s from cache: %w", key, err)
	}
	return h.ProcessValidatingWebhookConfigurationObject(vwc, false)
}

func (h *handler) ProcessValidatingWebhookConfigurationObject(vwc *admissionregistrationv1.ValidatingWebhookConfiguration, deleted bool) error {
	if vwc == nil {
		return nil
	}
	if deleted {
		h.correlator.ResolveByResource("validatingwebhookconfiguration", vwc.Name)
		return nil
	}

	hasService := func(ns, name string) bool {
		if h.serviceLister == nil {
			return true
		}
		_, err := h.serviceLister.Services(ns).Get(name)
		return err == nil
	}

	sigs := DetectValidatingWebhookIssue(vwc, hasService)
	for _, sig := range sigs {
		h.signalEvent(sig)
	}
	if len(sigs) == 0 {
		h.correlator.MarkResolved(correlation.BuildKey("", vwc.Name, "WebhookBackendNotFound", ""))
	}
	return nil
}

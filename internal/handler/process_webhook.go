package handler

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/constant"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
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
func DetectMutatingWebhookIssue(
	mwc *admissionregistrationv1.MutatingWebhookConfiguration,
	hasService func(ns, name string) bool,
) []*model.Observation {
	if mwc == nil || hasService == nil {
		return nil
	}
	var sigs []*model.Observation
	for _, w := range mwc.Webhooks {
		ref := serviceRef(w.ClientConfig.Service)
		if ref == "" {
			continue
		}
		svc := w.ClientConfig.Service
		ns, name := svc.Namespace, svc.Name
		if !hasService(ns, name) {
			sigs = append(sigs, observe.Object(
				"mutatingwebhookconfiguration", mwc,
				constant.ReasonWebhookBackendNotFound,
			).WithHint(fmt.Sprintf(
				"mutating webhook %q: service %s/%s does not exist",
				mwc.Name,
				ns,
				name,
			)))
		}
	}
	return sigs
}

// DetectValidatingWebhookIssue checks a ValidatingWebhookConfiguration for
// webhooks
// whose service backend doesn't exist.
func DetectValidatingWebhookIssue(
	vwc *admissionregistrationv1.ValidatingWebhookConfiguration,
	hasService func(ns, name string) bool,
) []*model.Observation {
	if vwc == nil || hasService == nil {
		return nil
	}
	var sigs []*model.Observation
	for _, w := range vwc.Webhooks {
		ref := serviceRef(w.ClientConfig.Service)
		if ref == "" {
			continue
		}
		svc := w.ClientConfig.Service
		ns, name := svc.Namespace, svc.Name
		if !hasService(ns, name) {
			sigs = append(sigs, observe.Object(
				"validatingwebhookconfiguration", vwc,
				constant.ReasonWebhookBackendNotFound,
			).WithHint(fmt.Sprintf(
				"validating webhook %q: service %s/%s does not exist",
				vwc.Name,
				ns,
				name,
			)))
		}
	}
	return sigs
}

func (h *handler) ProcessMutatingWebhookConfiguration(
	key string,
	deleted bool,
) error {
	if deleted {
		h.forgetWebhook("mutatingwebhookconfiguration", key)
		return nil
	}
	mwc, err := h.listers.MWC.Get(key)
	if err != nil {
		if errors.IsNotFound(err) {
			h.forgetWebhook("mutatingwebhookconfiguration", key)
			return nil
		}
		return fmt.Errorf(
			"failed to get mutatingwebhookconfiguration %s from cache: %w",
			key,
			err,
		)
	}
	return h.ProcessMutatingWebhookConfigurationObject(mwc, false)
}

func (h *handler) ProcessMutatingWebhookConfigurationObject(
	mwc *admissionregistrationv1.MutatingWebhookConfiguration,
	deleted bool,
) error {
	if mwc == nil {
		return nil
	}
	if deleted {
		h.forgetWebhook("mutatingwebhookconfiguration", mwc.Name)
		return nil
	}

	hasService := func(ns, name string) bool {
		if h.listers.Service == nil {
			return true // can't check, assume ok
		}
		_, err := h.listers.Service.Services(ns).Get(name)
		return err == nil
	}

	h.reconcile(
		model.NewObjectRef("mutatingwebhookconfiguration", "", mwc.Name),
		DetectMutatingWebhookIssue(mwc, hasService),
	)
	// The endpoint findings are about the webhook's backing Service, which is
	// a subject of its own -- resolving the configuration would not answer
	// for them.
	h.reconcile(
		model.NewObjectRef("webhook", mwc.Namespace, mwc.Name),
		h.detectWebhookEndpointIssues(
			mwc.Name, mwc.Namespace, mwc.Labels,
			MutatingWebhookServices(mwc),
		),
	)
	return nil
}

func (h *handler) ProcessValidatingWebhookConfiguration(
	key string,
	deleted bool,
) error {
	if deleted {
		h.forgetWebhook("validatingwebhookconfiguration", key)
		return nil
	}
	vwc, err := h.listers.VWC.Get(key)
	if err != nil {
		if errors.IsNotFound(err) {
			h.forgetWebhook("validatingwebhookconfiguration", key)
			return nil
		}
		return fmt.Errorf(
			"failed to get validatingwebhookconfiguration %s from cache: %w",
			key,
			err,
		)
	}
	return h.ProcessValidatingWebhookConfigurationObject(vwc, false)
}

func (h *handler) ProcessValidatingWebhookConfigurationObject(
	vwc *admissionregistrationv1.ValidatingWebhookConfiguration,
	deleted bool,
) error {
	if vwc == nil {
		return nil
	}
	if deleted {
		h.forgetWebhook("validatingwebhookconfiguration", vwc.Name)
		return nil
	}

	hasService := func(ns, name string) bool {
		if h.listers.Service == nil {
			return true
		}
		_, err := h.listers.Service.Services(ns).Get(name)
		return err == nil
	}

	h.reconcile(
		model.NewObjectRef("validatingwebhookconfiguration", "", vwc.Name),
		DetectValidatingWebhookIssue(vwc, hasService),
	)
	h.reconcile(
		model.NewObjectRef("webhook", vwc.Namespace, vwc.Name),
		h.detectWebhookEndpointIssues(
			vwc.Name, vwc.Namespace, vwc.Labels,
			ValidatingWebhookServices(vwc),
		),
	)
	return nil
}

// forgetWebhook retires both subjects a webhook configuration reports under:
// the configuration itself, and the backing Service its endpoint findings are
// about.
func (h *handler) forgetWebhook(kind, name string) {
	h.reconcileGone(model.NewObjectRef(kind, "", name))
	h.reconcileGone(model.NewObjectRef("webhook", "", name))
}

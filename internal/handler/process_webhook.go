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
	findings, err := DetectMutatingWebhookIssueWithLookup(
		mwc,
		func(ns, name string) (bool, error) {
			return hasService(ns, name), nil
		},
	)
	if err != nil {
		return nil
	}
	return findings
}

// DetectMutatingWebhookIssueWithLookup is the error-aware detector used by
// live processing. NotFound is a finding; every other lookup error is unknown.
func DetectMutatingWebhookIssueWithLookup(
	mwc *admissionregistrationv1.MutatingWebhookConfiguration,
	lookup ServiceLookup,
) ([]*model.Observation, error) {
	if mwc == nil || lookup == nil {
		return nil, nil
	}
	var sigs []*model.Observation
	for _, w := range mwc.Webhooks {
		ref := serviceRef(w.ClientConfig.Service)
		if ref == "" {
			continue
		}
		svc := w.ClientConfig.Service
		ns, name := svc.Namespace, svc.Name
		exists, err := lookup(ns, name)
		if err != nil {
			return nil, fmt.Errorf(
				"lookup mutating webhook Service %s/%s: %w",
				ns,
				name,
				err,
			)
		}
		if !exists {
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
	return sigs, nil
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
	findings, err := DetectValidatingWebhookIssueWithLookup(
		vwc,
		func(ns, name string) (bool, error) {
			return hasService(ns, name), nil
		},
	)
	if err != nil {
		return nil
	}
	return findings
}

// DetectValidatingWebhookIssueWithLookup is the error-aware detector used by
// live processing. NotFound is a finding; every other lookup error is unknown.
func DetectValidatingWebhookIssueWithLookup(
	vwc *admissionregistrationv1.ValidatingWebhookConfiguration,
	lookup ServiceLookup,
) ([]*model.Observation, error) {
	if vwc == nil || lookup == nil {
		return nil, nil
	}
	var sigs []*model.Observation
	for _, w := range vwc.Webhooks {
		ref := serviceRef(w.ClientConfig.Service)
		if ref == "" {
			continue
		}
		svc := w.ClientConfig.Service
		ns, name := svc.Namespace, svc.Name
		exists, err := lookup(ns, name)
		if err != nil {
			return nil, fmt.Errorf(
				"lookup validating webhook Service %s/%s: %w",
				ns,
				name,
				err,
			)
		}
		if !exists {
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
	return sigs, nil
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

	serviceFindings, err := DetectMutatingWebhookIssueWithLookup(
		mwc,
		h.serviceLookup(),
	)
	if err != nil {
		return fmt.Errorf("evaluate mutating webhook %s: %w", mwc.Name, err)
	}
	endpointFindings, err := h.detectWebhookEndpointIssues(
		mwc.Name, mwc.Namespace, mwc.Labels,
		MutatingWebhookServices(mwc),
	)
	if err != nil {
		return fmt.Errorf("evaluate mutating webhook endpoints %s: %w", mwc.Name, err)
	}

	h.reconcile(
		model.NewObjectRef("mutatingwebhookconfiguration", "", mwc.Name),
		serviceFindings,
	)
	// The endpoint findings are about the webhook's backing Service, which is
	// a subject of its own -- resolving the configuration would not answer
	// for them.
	h.reconcile(
		model.NewObjectRef("webhook", mwc.Namespace, mwc.Name),
		endpointFindings,
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

	serviceFindings, err := DetectValidatingWebhookIssueWithLookup(
		vwc,
		h.serviceLookup(),
	)
	if err != nil {
		return fmt.Errorf("evaluate validating webhook %s: %w", vwc.Name, err)
	}
	endpointFindings, err := h.detectWebhookEndpointIssues(
		vwc.Name, vwc.Namespace, vwc.Labels,
		ValidatingWebhookServices(vwc),
	)
	if err != nil {
		return fmt.Errorf(
			"evaluate validating webhook endpoints %s: %w",
			vwc.Name,
			err,
		)
	}

	h.reconcile(
		model.NewObjectRef("validatingwebhookconfiguration", "", vwc.Name),
		serviceFindings,
	)
	h.reconcile(
		model.NewObjectRef("webhook", vwc.Namespace, vwc.Name),
		endpointFindings,
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

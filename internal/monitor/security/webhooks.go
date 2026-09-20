package security

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServiceLookup reports whether an admission Service exists.
type ServiceLookup func(namespace, name string) (bool, error)

// DetectMutatingWebhookIssue adapts a boolean Service lookup.
func DetectMutatingWebhookIssue(
	configuration *admissionregistrationv1.MutatingWebhookConfiguration,
	hasService func(string, string) bool,
) []*model.Observation {
	if configuration == nil || hasService == nil {
		return nil
	}
	findings, err := DetectMutatingWebhookIssueWithLookup(
		configuration,
		func(namespace, name string) (bool, error) {
			return hasService(namespace, name), nil
		},
	)
	if err != nil {
		return nil
	}
	return findings
}

// DetectMutatingWebhookIssueWithLookup reports missing Service backends.
func DetectMutatingWebhookIssueWithLookup(
	configuration *admissionregistrationv1.MutatingWebhookConfiguration,
	lookup ServiceLookup,
) ([]*model.Observation, error) {
	if configuration == nil || lookup == nil {
		return nil, nil
	}
	var findings []*model.Observation
	for _, webhook := range configuration.Webhooks {
		if webhook.ClientConfig.Service == nil {
			continue
		}
		service := webhook.ClientConfig.Service
		exists, err := lookup(service.Namespace, service.Name)
		if err != nil {
			return nil, fmt.Errorf(
				"lookup mutating webhook Service %s/%s: %w",
				service.Namespace,
				service.Name,
				err,
			)
		}
		if !exists {
			findings = append(findings, webhookObservation(
				configuration,
				"mutatingwebhookconfiguration",
				configuration.Name,
				service.Namespace,
				service.Name,
				constant.ReasonWebhookBackendNotFound,
				"mutating",
			))
		}
	}
	return findings, nil
}

// DetectValidatingWebhookIssue adapts a boolean Service lookup.
func DetectValidatingWebhookIssue(
	configuration *admissionregistrationv1.ValidatingWebhookConfiguration,
	hasService func(string, string) bool,
) []*model.Observation {
	if configuration == nil || hasService == nil {
		return nil
	}
	findings, err := DetectValidatingWebhookIssueWithLookup(
		configuration,
		func(namespace, name string) (bool, error) {
			return hasService(namespace, name), nil
		},
	)
	if err != nil {
		return nil
	}
	return findings
}

// DetectValidatingWebhookIssueWithLookup reports missing Service backends.
func DetectValidatingWebhookIssueWithLookup(
	configuration *admissionregistrationv1.ValidatingWebhookConfiguration,
	lookup ServiceLookup,
) ([]*model.Observation, error) {
	if configuration == nil || lookup == nil {
		return nil, nil
	}
	var findings []*model.Observation
	for _, webhook := range configuration.Webhooks {
		if webhook.ClientConfig.Service == nil {
			continue
		}
		service := webhook.ClientConfig.Service
		exists, err := lookup(service.Namespace, service.Name)
		if err != nil {
			return nil, fmt.Errorf(
				"lookup validating webhook Service %s/%s: %w",
				service.Namespace,
				service.Name,
				err,
			)
		}
		if !exists {
			findings = append(findings, webhookObservation(
				configuration,
				"validatingwebhookconfiguration",
				configuration.Name,
				service.Namespace,
				service.Name,
				constant.ReasonWebhookBackendNotFound,
				"validating",
			))
		}
	}
	return findings, nil
}

func webhookObservation(
	object metav1.Object,
	kind, name, namespace, serviceName, reason, webhookType string,
) *model.Observation {
	return observe.Object(kind, object, reason).WithHint(fmt.Sprintf(
		"%s webhook %q: service %s/%s does not exist",
		webhookType,
		name,
		namespace,
		serviceName,
	))
}

package security

import (
	"fmt"
	"sync"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	admv1lister "k8s.io/client-go/listers/admissionregistration/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	discoveryv1lister "k8s.io/client-go/listers/discovery/v1"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/monitor"
)

// Runtime owns queue processing for admission webhook configurations. The
// endpoint findings intentionally use a separate webhook subject from the
// configuration subject.
type Runtime struct {
	runtime      config.RuntimeConfig
	sink         monitor.ReconciliationSink
	mu           sync.RWMutex
	started      bool
	mwc          admv1lister.MutatingWebhookConfigurationLister
	vwc          admv1lister.ValidatingWebhookConfigurationLister
	services     corev1lister.ServiceLister
	endpointList discoveryv1lister.EndpointSliceLister
	configured   bool
}

// ConfigureSources wires admission, Service, and EndpointSlice sources once.
func (r *Runtime) ConfigureSources(sources Sources) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return fmt.Errorf("security sources cannot change after processing starts")
	}
	if r.configured {
		return fmt.Errorf("security sources are already configured")
	}
	r.configured = true
	r.mwc, r.vwc = sources.MutatingWebhooks, sources.ValidatingWebhooks
	r.services = sources.Services
	r.endpointList = sources.EndpointSlices
	return nil
}

// beginProcessing closes the source configuration window. The controller
// wires all admission sources before queue workers begin.
func (r *Runtime) beginProcessing() {
	r.mu.Lock()
	r.started = true
	r.mu.Unlock()
}

// ProcessMutatingWebhookConfiguration evaluates one queue key.
func (r *Runtime) ProcessMutatingWebhookConfiguration(
	key string, deleted bool,
) error {
	r.beginProcessing()
	if !r.mutatingListerAvailable() {
		return nil
	}
	if deleted {
		r.forget("mutatingwebhookconfiguration", key)
		return nil
	}
	r.mu.RLock()
	lister := r.mwc
	r.mu.RUnlock()
	if lister == nil {
		return nil
	}
	configuration, err := lister.Get(key)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.forget("mutatingwebhookconfiguration", key)
			return nil
		}
		return fmt.Errorf("get mutating webhook %s from cache: %w", key, err)
	}
	return r.processMutating(configuration)
}

// ProcessValidatingWebhookConfiguration evaluates one queue key.
func (r *Runtime) ProcessValidatingWebhookConfiguration(
	key string, deleted bool,
) error {
	r.beginProcessing()
	if !r.validatingListerAvailable() {
		return nil
	}
	if deleted {
		r.forget("validatingwebhookconfiguration", key)
		return nil
	}
	r.mu.RLock()
	lister := r.vwc
	r.mu.RUnlock()
	if lister == nil {
		return nil
	}
	configuration, err := lister.Get(key)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.forget("validatingwebhookconfiguration", key)
			return nil
		}
		return fmt.Errorf("get validating webhook %s from cache: %w", key, err)
	}
	return r.processValidating(configuration)
}

func (r *Runtime) processMutating(
	configuration *admissionregistrationv1.MutatingWebhookConfiguration,
) error {
	if r.serviceListerAvailable() {
		serviceFindings, err := DetectMutatingWebhookIssueWithLookup(
			configuration, r.serviceExists,
		)
		if err != nil {
			return fmt.Errorf(
				"evaluate mutating webhook %s: %w", configuration.Name, err,
			)
		}
		r.reconcile(
			model.NewObjectRef(
				"mutatingwebhookconfiguration", "", configuration.Name,
			), serviceFindings,
		)
	}
	if r.endpointListerAvailable() {
		endpointFindings, err := r.endpointFindings(
			configuration.Name, configuration.Namespace, configuration.Labels,
			MutatingWebhookServices(configuration),
		)
		if err != nil {
			return fmt.Errorf(
				"evaluate mutating webhook endpoints %s: %w",
				configuration.Name, err,
			)
		}
		r.reconcile(
			model.NewObjectRef("webhook", configuration.Namespace, configuration.Name),
			endpointFindings,
		)
	}
	return nil
}

func (r *Runtime) processValidating(
	configuration *admissionregistrationv1.ValidatingWebhookConfiguration,
) error {
	if r.serviceListerAvailable() {
		serviceFindings, err := DetectValidatingWebhookIssueWithLookup(
			configuration, r.serviceExists,
		)
		if err != nil {
			return fmt.Errorf(
				"evaluate validating webhook %s: %w", configuration.Name, err,
			)
		}
		r.reconcile(
			model.NewObjectRef(
				"validatingwebhookconfiguration", "", configuration.Name,
			), serviceFindings,
		)
	}
	if r.endpointListerAvailable() {
		endpointFindings, err := r.endpointFindings(
			configuration.Name, configuration.Namespace, configuration.Labels,
			ValidatingWebhookServices(configuration),
		)
		if err != nil {
			return fmt.Errorf(
				"evaluate validating webhook endpoints %s: %w",
				configuration.Name, err,
			)
		}
		r.reconcile(
			model.NewObjectRef("webhook", configuration.Namespace, configuration.Name),
			endpointFindings,
		)
	}
	return nil
}

func (r *Runtime) serviceListerAvailable() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.services != nil
}

func (r *Runtime) endpointListerAvailable() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.endpointList != nil
}

func (r *Runtime) mutatingListerAvailable() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.mwc != nil
}

func (r *Runtime) validatingListerAvailable() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.vwc != nil
}

func (r *Runtime) serviceExists(namespace, name string) (bool, error) {
	r.mu.RLock()
	lister := r.services
	r.mu.RUnlock()
	if lister == nil {
		return false, nil
	}
	_, err := lister.Services(namespace).Get(name)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

func (r *Runtime) endpointFindings(
	name, namespace string,
	labelsMap map[string]string,
	refs []*admissionregistrationv1.ServiceReference,
) ([]*model.Observation, error) {
	r.mu.RLock()
	lister := r.endpointList
	r.mu.RUnlock()
	return DetectWebhookEndpointIssuesWithError(
		lister, name, namespace, labelsMap, refs,
	)
}

func (r *Runtime) reconcile(
	subject model.ObjectRef, findings []*model.Observation,
) {
	if r.sink == nil {
		return
	}
	for _, finding := range findings {
		if finding != nil {
			finding.IncludeEvents = r.runtime.IncludeEvents()
			finding.IncludeLogs = r.runtime.IncludeLogs()
		}
	}
	r.sink.Reconcile(subject, findings)
}

func (r *Runtime) forget(kind, name string) {
	if r.sink == nil {
		return
	}
	if r.serviceListerAvailable() {
		r.sink.ReconcileGone(model.NewObjectRef(kind, "", name))
	}
	if r.endpointListerAvailable() {
		r.sink.ReconcileGone(model.NewObjectRef("webhook", "", name))
	}
}

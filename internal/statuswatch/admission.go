package statuswatch

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/k8s"
)

func (m *Monitor) startAdmissionInformers(
	factory dynamicinformer.DynamicSharedInformerFactory,
) {
	policyInformer := factory.ForResource(validatingAdmissionPolicyGVR).Informer()
	if err := policyInformer.SetTransform(k8s.TrimManagedFields); err != nil {
		klog.ErrorS(err, "statuswatch: set policy cache transform")
		return
	}
	if _, err := policyInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: m.processAdmissionPolicy,
		UpdateFunc: func(_, obj interface{}) {
			m.processAdmissionPolicy(obj)
		},
		DeleteFunc: m.deleteAdmissionPolicy,
	}); err != nil {
		klog.ErrorS(err, "statuswatch: register admission policy informer")
	}
	bindingInformer := factory.
		ForResource(validatingAdmissionBindingGVR).Informer()
	if err := bindingInformer.SetTransform(k8s.TrimManagedFields); err != nil {
		klog.ErrorS(err, "statuswatch: set binding cache transform")
		return
	}
	if _, err := bindingInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: m.processAdmissionBinding,
		UpdateFunc: func(_, obj interface{}) {
			m.processAdmissionBinding(obj)
		},
		DeleteFunc: m.deleteAdmissionBinding,
	}); err != nil {
		klog.ErrorS(err, "statuswatch: register admission policy binding informer")
	}
}

func (m *Monitor) processAdmissionPolicy(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	m.mu.Lock()
	m.admissionPolicies[u.GetName()] = struct{}{}
	m.mu.Unlock()
	if sig := admissionPolicySignal(u); sig != nil {
		m.correlator.Process(
			event.Event{
				Resource: sig.Resource, PodName: sig.PodName,
				Reason: sig.Reason, Hint: sig.Hint, Labels: sig.Labels,
			}, sig.Owner, nil,
		)
	} else {
		m.resolve("", u.GetName(), constant.ReasonAdmissionPolicyInvalid)
	}
	m.recheckAdmissionBindings()
}

func (m *Monitor) processAdmissionBinding(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	m.mu.Lock()
	m.admissionBindings[u.GetName()] = u.DeepCopy()
	m.mu.Unlock()
	m.processAdmissionBindingObject(u)
}

func (m *Monitor) processAdmissionBindingObject(u *unstructured.Unstructured) {
	policy, _, _ := unstructured.NestedString(u.Object, "spec", "policyName")
	m.mu.Lock()
	_, exists := m.admissionPolicies[policy]
	m.mu.Unlock()
	if policy != "" && !exists {
		sig := &event.Signal{
			Resource: "validatingadmissionpolicybinding",
			PodName:  u.GetName(), Owner: u.GetName(),
			Reason: constant.ReasonAdmissionBindingInvalid,
			Labels: u.GetLabels(),
			Hint: fmt.Sprintf(
				"binding references missing ValidatingAdmissionPolicy %q", policy,
			),
		}
		m.correlator.Process(
			event.Event{
				Resource: sig.Resource, Reason: sig.Reason,
				Hint: sig.Hint, Labels: sig.Labels,
			}, sig.Owner, nil,
		)
		return
	}
	m.resolve("", u.GetName(), constant.ReasonAdmissionBindingInvalid)
}

func (m *Monitor) recheckAdmissionBindings() {
	m.mu.Lock()
	bindings := make([]*unstructured.Unstructured, 0, len(m.admissionBindings))
	for _, binding := range m.admissionBindings {
		bindings = append(bindings, binding.DeepCopy())
	}
	m.mu.Unlock()
	for _, binding := range bindings {
		m.processAdmissionBindingObject(binding)
	}
}

func (m *Monitor) deleteAdmissionPolicy(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	m.mu.Lock()
	delete(m.admissionPolicies, u.GetName())
	m.mu.Unlock()
	m.resolve("", u.GetName(), constant.ReasonAdmissionPolicyInvalid)
	m.recheckAdmissionBindings()
}

func (m *Monitor) deleteAdmissionBinding(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	m.mu.Lock()
	delete(m.admissionBindings, u.GetName())
	m.mu.Unlock()
	m.resolve("", u.GetName(), constant.ReasonAdmissionBindingInvalid)
}

func admissionPolicySignal(u *unstructured.Unstructured) *event.Signal {
	if warnings, found, _ := unstructured.NestedSlice(
		u.Object, "status", "typeChecking", "expressionWarnings",
	); found && len(warnings) > 0 {
		return &event.Signal{
			Resource: "validatingadmissionpolicy",
			PodName:  u.GetName(), Owner: u.GetName(),
			Reason: constant.ReasonAdmissionPolicyInvalid,
			Labels: u.GetLabels(),
			Hint: fmt.Sprintf(
				"type checking reported %d expression warning(s): %s",
				len(warnings), admissionWarningText(warnings),
			),
		}
	}
	sig := failureSignal(
		u, "validatingadmissionpolicy", u.GetName(),
		defaultConditionRulesWithAdmission(),
	)
	if sig != nil {
		sig.Reason = constant.ReasonAdmissionPolicyInvalid
	}
	return sig
}

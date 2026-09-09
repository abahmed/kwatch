package statuswatch

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

func (m *Monitor) startAdmissionInformers(
	factory dynamicinformer.DynamicSharedInformerFactory,
) {
	if m.resourceAvailable(validatingAdmissionPolicyGVR) {
		policyInformer := factory.
			ForResource(validatingAdmissionPolicyGVR).Informer()
		if err := policyInformer.SetTransform(k8s.TrimManagedFields); err != nil {
			klog.ErrorS(err, "statuswatch: set policy cache transform")
			return
		}
		if _, err := policyInformer.AddEventHandler(
			cache.ResourceEventHandlerFuncs{
				AddFunc: m.processAdmissionPolicy,
				UpdateFunc: func(_, obj interface{}) {
					m.processAdmissionPolicy(obj)
				},
				DeleteFunc: m.deleteAdmissionPolicy,
			},
		); err != nil {
			klog.ErrorS(err, "statuswatch: register admission policy informer")
		}
	}
	if !m.resourceAvailable(validatingAdmissionBindingGVR) {
		return
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
		m.correlator.Process(sig)
	} else {
		m.resolve(
			"validatingadmissionpolicy", "", u.GetName(),
			constant.ReasonAdmissionPolicyInvalid,
		)
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
		// The hand-built event here also dropped the subject's name, so the
		// incident never said which binding was broken.
		m.correlator.Process(
			observe.ClusterObject(
				"validatingadmissionpolicybinding", u.GetName(),
				constant.ReasonAdmissionBindingInvalid,
			).WithLabels(u.GetLabels()).WithHint(fmt.Sprintf(
				"binding references missing ValidatingAdmissionPolicy %q",
				policy,
			)),
		)
		return
	}
	m.resolve(
		"validatingadmissionpolicybinding", "", u.GetName(),
		constant.ReasonAdmissionBindingInvalid,
	)
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
	m.resolve(
		"validatingadmissionpolicy", "", u.GetName(),
		constant.ReasonAdmissionPolicyInvalid,
	)
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
	m.resolve(
		"validatingadmissionpolicybinding", "", u.GetName(),
		constant.ReasonAdmissionBindingInvalid,
	)
}

func admissionPolicySignal(
	u *unstructured.Unstructured,
) *model.Observation {
	if warnings, found, _ := unstructured.NestedSlice(
		u.Object, "status", "typeChecking", "expressionWarnings",
	); found && len(warnings) > 0 {
		return observe.ClusterObject(
			"validatingadmissionpolicy", u.GetName(),
			constant.ReasonAdmissionPolicyInvalid,
		).WithLabels(u.GetLabels()).WithHint(fmt.Sprintf(
			"type checking reported %d expression warning(s): %s",
			len(warnings), admissionWarningText(warnings),
		))
	}
	sig := failureSignal(
		u, "validatingadmissionpolicy",
		defaultConditionRulesWithAdmission(),
	)
	if sig != nil {
		sig.Reason = constant.ReasonAdmissionPolicyInvalid
	}
	return sig
}

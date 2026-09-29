package kube

import (
	"sort"
	"strings"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	storagev1 "k8s.io/api/storage/v1"

	"github.com/abahmed/kwatch/internal/knowledge"
)

// Entity kinds for policy and admission objects.
const (
	KindPDB              knowledge.Kind = "poddisruptionbudget"
	KindQuota            knowledge.Kind = "resourcequota"
	KindNetworkPolicy    knowledge.Kind = "networkpolicy"
	KindVolumeAttachment knowledge.Kind = "volumeattachment"
	KindMutatingWebhook  knowledge.Kind = "mutatingwebhookconfiguration"
	KindValidatingHook   knowledge.Kind = "validatingwebhookconfiguration"
)

// Attribute names for policy objects.
const (
	AttrDisruptionsAllowed = "disruptions.allowed"
	AttrCurrentHealthy     = "healthy.current"
	AttrDesiredHealthy     = "healthy.desired"
	AttrExpectedPods       = "pods.expected"
	AttrExhausted          = "quota.exhausted"
	AttrAttached           = "attached"
	AttrAttachError        = "attach.error"
	AttrFailurePolicy      = "failure.policy"
	AttrPolicyDigest       = "policy.digest"
)

// PDBSchema describes PodDisruptionBudgets.
type PDBSchema struct{}

// Kind implements Schema.
func (PDBSchema) Kind() knowledge.Kind { return KindPDB }

// RelationTypes implements Schema.
func (PDBSchema) RelationTypes() []knowledge.RelationType { return nil }

// Describe implements Schema.
func (PDBSchema) Describe(obj any) (Description, bool) {
	pdb, ok := obj.(*policyv1.PodDisruptionBudget)
	if !ok {
		return Description{}, false
	}
	st := pdb.Status
	return Description{
		ID: objectID(KindPDB, pdb), UID: string(pdb.UID),
		Attributes: map[string]knowledge.Value{
			AttrSelector:           knowledge.Text(selectorText(pdb.Spec.Selector)),
			AttrDisruptionsAllowed: number(st.DisruptionsAllowed),
			AttrCurrentHealthy:     number(st.CurrentHealthy),
			AttrDesiredHealthy:     number(st.DesiredHealthy),
			AttrExpectedPods:       number(st.ExpectedPods),
		},
	}, true
}

// Diff implements Schema.
func (PDBSchema) Diff(_, _ any) []knowledge.FieldChange { return nil }

// QuotaSchema describes ResourceQuotas; a quota constrains its namespace.
type QuotaSchema struct{}

// Kind implements Schema.
func (QuotaSchema) Kind() knowledge.Kind { return KindQuota }

// RelationTypes implements Schema.
func (QuotaSchema) RelationTypes() []knowledge.RelationType {
	return []knowledge.RelationType{knowledge.Constrains}
}

// Describe implements Schema.
func (QuotaSchema) Describe(obj any) (Description, bool) {
	quota, ok := obj.(*corev1.ResourceQuota)
	if !ok {
		return Description{}, false
	}
	var exhausted []string
	for name, hard := range quota.Status.Hard {
		if used, ok := quota.Status.Used[name]; ok && used.Cmp(hard) >= 0 {
			exhausted = append(exhausted, string(name))
		}
	}
	sort.Strings(exhausted)
	rel := relations{}
	rel.add(knowledge.Constrains,
		knowledge.NewEntityID(KindNamespace, "", quota.Namespace))
	return Description{
		ID: objectID(KindQuota, quota), UID: string(quota.UID),
		Attributes: map[string]knowledge.Value{
			AttrExhausted: knowledge.Text(strings.Join(exhausted, ",")),
		},
		Relations: rel,
	}, true
}

// Diff implements Schema.
func (QuotaSchema) Diff(old, new any) []knowledge.FieldChange {
	before, ok1 := old.(*corev1.ResourceQuota)
	after, ok2 := new.(*corev1.ResourceQuota)
	if !ok1 || !ok2 {
		return nil
	}
	b, a := resourceListText(before.Spec.Hard), resourceListText(after.Spec.Hard)
	if b == a {
		return nil
	}
	return []knowledge.FieldChange{{Path: "spec.hard", Before: b, After: a}}
}

// NetworkPolicySchema describes NetworkPolicies. Its changes matter: a
// policy edit shortly before connection failures of the pods it selects is
// a root-cause candidate.
type NetworkPolicySchema struct{}

// Kind implements Schema.
func (NetworkPolicySchema) Kind() knowledge.Kind { return KindNetworkPolicy }

// RelationTypes implements Schema.
func (NetworkPolicySchema) RelationTypes() []knowledge.RelationType {
	return nil
}

// Describe implements Schema.
func (NetworkPolicySchema) Describe(obj any) (Description, bool) {
	np, ok := obj.(*networkingv1.NetworkPolicy)
	if !ok {
		return Description{}, false
	}
	return Description{
		ID: objectID(KindNetworkPolicy, np), UID: string(np.UID),
		Attributes: map[string]knowledge.Value{
			AttrSelector: knowledge.Text(selectorText(&np.Spec.PodSelector)),
			AttrPolicyDigest: knowledge.Text(
				digest([]byte(np.Spec.String()))),
		},
	}, true
}

// Diff implements Schema.
func (NetworkPolicySchema) Diff(old, new any) []knowledge.FieldChange {
	before, ok1 := old.(*networkingv1.NetworkPolicy)
	after, ok2 := new.(*networkingv1.NetworkPolicy)
	if !ok1 || !ok2 {
		return nil
	}
	b := digest([]byte(before.Spec.String()))
	a := digest([]byte(after.Spec.String()))
	if b == a {
		return nil
	}
	return []knowledge.FieldChange{{Path: "spec", Before: b, After: a}}
}

// VolumeAttachmentSchema describes CSI volume attachments.
type VolumeAttachmentSchema struct{}

// Kind implements Schema.
func (VolumeAttachmentSchema) Kind() knowledge.Kind {
	return KindVolumeAttachment
}

// RelationTypes implements Schema.
func (VolumeAttachmentSchema) RelationTypes() []knowledge.RelationType {
	return []knowledge.RelationType{knowledge.References, knowledge.RunsOn}
}

// Describe implements Schema.
func (VolumeAttachmentSchema) Describe(obj any) (Description, bool) {
	va, ok := obj.(*storagev1.VolumeAttachment)
	if !ok {
		return Description{}, false
	}
	attrs := map[string]knowledge.Value{
		AttrAttached: knowledge.Bool(va.Status.Attached),
	}
	if err := va.Status.AttachError; err != nil {
		attrs[AttrAttachError] = knowledge.Text(truncate(err.Message))
	}
	rel := relations{}
	rel.add(knowledge.RunsOn,
		knowledge.NewEntityID(KindNode, "", va.Spec.NodeName))
	if pv := va.Spec.Source.PersistentVolumeName; pv != nil {
		rel.add(knowledge.References, knowledge.NewEntityID(KindPV, "", *pv))
	}
	return Description{
		ID: objectID(KindVolumeAttachment, va), UID: string(va.UID),
		Attributes: attrs, Relations: rel,
	}, true
}

// Diff implements Schema.
func (VolumeAttachmentSchema) Diff(_, _ any) []knowledge.FieldChange {
	return nil
}

// WebhookSchema describes admission webhook configurations. Each webhook
// that calls an in-cluster Service is served by it.
type WebhookSchema struct{ Mutating bool }

// Kind implements Schema.
func (w WebhookSchema) Kind() knowledge.Kind {
	if w.Mutating {
		return KindMutatingWebhook
	}
	return KindValidatingHook
}

// RelationTypes implements Schema.
func (WebhookSchema) RelationTypes() []knowledge.RelationType {
	return []knowledge.RelationType{knowledge.Serves}
}

// Describe implements Schema.
func (w WebhookSchema) Describe(obj any) (Description, bool) {
	var (
		name     string
		uid      string
		clients  []admissionv1.WebhookClientConfig
		policies []string
	)
	switch config := obj.(type) {
	case *admissionv1.MutatingWebhookConfiguration:
		if !w.Mutating {
			return Description{}, false
		}
		name, uid = config.Name, string(config.UID)
		for _, hook := range config.Webhooks {
			clients = append(clients, hook.ClientConfig)
			policies = append(policies, failurePolicy(hook.FailurePolicy))
		}
	case *admissionv1.ValidatingWebhookConfiguration:
		if w.Mutating {
			return Description{}, false
		}
		name, uid = config.Name, string(config.UID)
		for _, hook := range config.Webhooks {
			clients = append(clients, hook.ClientConfig)
			policies = append(policies, failurePolicy(hook.FailurePolicy))
		}
	default:
		return Description{}, false
	}
	rel := relations{}
	for _, client := range clients {
		if svc := client.Service; svc != nil {
			rel.add(knowledge.Serves, knowledge.NewEntityID(
				KindService, svc.Namespace, svc.Name))
		}
	}
	return Description{
		ID: knowledge.NewEntityID(w.Kind(), "", name), UID: uid,
		Attributes: map[string]knowledge.Value{
			AttrFailurePolicy: knowledge.Text(strings.Join(policies, ",")),
		},
		Relations: rel,
	}, true
}

// Diff implements Schema.
func (WebhookSchema) Diff(_, _ any) []knowledge.FieldChange { return nil }

func failurePolicy(policy *admissionv1.FailurePolicyType) string {
	if policy == nil {
		return string(admissionv1.Fail)
	}
	return string(*policy)
}

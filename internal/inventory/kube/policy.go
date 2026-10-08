package kube

import (
	"sort"
	"strings"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	storagev1 "k8s.io/api/storage/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Entity kinds for policy and admission objects.
const (
	KindPDB              inventory.Kind = "poddisruptionbudget"
	KindQuota            inventory.Kind = "resourcequota"
	KindNetworkPolicy    inventory.Kind = "networkpolicy"
	KindVolumeAttachment inventory.Kind = "volumeattachment"
	KindMutatingWebhook  inventory.Kind = "mutatingwebhookconfiguration"
	KindValidatingHook   inventory.Kind = "validatingwebhookconfiguration"
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
	AttrDetachError        = "detach.error"
	AttrFailurePolicy      = "failure.policy"
	// AttrWebhookNames lists a configuration's webhook names, comma
	// separated. The API server names the webhook it failed to call
	// ("failed calling webhook \"x.example.com\""), which is how a
	// failed create is traced back to its configuration.
	AttrWebhookNames = "webhook.names"
	AttrPolicyDigest = "policy.digest"
	AttrDeniesEgress = "denies.all.egress"
)

// PDBSchema describes PodDisruptionBudgets.
type PDBSchema struct{}

// Kind implements Schema.
func (PDBSchema) Kind() inventory.Kind { return KindPDB }

// RelationTypes implements Schema.
func (PDBSchema) RelationTypes() []inventory.RelationType { return nil }

// Describe implements Schema.
func (PDBSchema) Describe(obj any) (Description, bool) {
	pdb, ok := obj.(*policyv1.PodDisruptionBudget)
	if !ok {
		return Description{}, false
	}
	st := pdb.Status
	attrs := map[string]inventory.Value{
		AttrSelector:           inventory.Text(selectorText(pdb.Spec.Selector)),
		AttrDisruptionsAllowed: number(st.DisruptionsAllowed),
		AttrCurrentHealthy:     number(st.CurrentHealthy),
		AttrDesiredHealthy:     number(st.DesiredHealthy),
		AttrExpectedPods:       number(st.ExpectedPods),
	}
	setConditions(attrs, metaConditions(st.Conditions))
	return Description{
		ID: objectID(KindPDB, pdb), UID: string(pdb.UID),
		Attributes: attrs,
	}, true
}

// Diff implements Schema.
func (PDBSchema) Diff(old, new any) []inventory.FieldChange {
	before, ok1 := old.(*policyv1.PodDisruptionBudget)
	after, ok2 := new.(*policyv1.PodDisruptionBudget)
	if !ok1 || !ok2 {
		return nil
	}
	return pdbChanges(before, after)
}

// QuotaSchema describes ResourceQuotas; a quota constrains its namespace.
type QuotaSchema struct{}

// Kind implements Schema.
func (QuotaSchema) Kind() inventory.Kind { return KindQuota }

// RelationTypes implements Schema.
func (QuotaSchema) RelationTypes() []inventory.RelationType {
	return []inventory.RelationType{inventory.Constrains}
}

// Describe implements Schema.
func (QuotaSchema) Describe(obj any) (Description, bool) {
	quota, ok := obj.(*corev1.ResourceQuota)
	if !ok {
		return Description{}, false
	}
	var exhausted, zero []string
	for name, hard := range quota.Status.Hard {
		if used, ok := quota.Status.Used[name]; ok && used.Cmp(hard) >= 0 {
			exhausted = append(exhausted, string(name))
		}
		if hard.IsZero() {
			zero = append(zero, string(name))
		}
	}
	sort.Strings(exhausted)
	sort.Strings(zero)
	rel := relations{}
	rel.add(inventory.Constrains,
		inventory.CoreID(KindNamespace, "", quota.Namespace))
	return Description{
		ID: objectID(KindQuota, quota), UID: string(quota.UID),
		Attributes: map[string]inventory.Value{
			AttrExhausted:     inventory.Text(strings.Join(exhausted, ",")),
			AttrQuotaZeroHard: inventory.Text(strings.Join(zero, ",")),
			AttrQuotaNearLimit: inventory.Text(
				quotaNearLimit(quota.Status)),
		},
		Relations: rel,
	}, true
}

// Diff implements Schema.
func (QuotaSchema) Diff(old, new any) []inventory.FieldChange {
	before, ok1 := old.(*corev1.ResourceQuota)
	after, ok2 := new.(*corev1.ResourceQuota)
	if !ok1 || !ok2 {
		return nil
	}
	b, a := resourceListText(before.Spec.Hard), resourceListText(after.Spec.Hard)
	if b == a {
		return nil
	}
	return []inventory.FieldChange{{Path: "spec.hard", Before: b, After: a}}
}

// NetworkPolicySchema describes NetworkPolicies. Its changes matter: a
// policy edit shortly before connection failures of the pods it selects is
// a root-cause candidate.
type NetworkPolicySchema struct{}

// Kind implements Schema.
func (NetworkPolicySchema) Kind() inventory.Kind { return KindNetworkPolicy }

// RelationTypes implements Schema.
func (NetworkPolicySchema) RelationTypes() []inventory.RelationType {
	return nil
}

// Describe implements Schema.
func (NetworkPolicySchema) Describe(obj any) (Description, bool) {
	np, ok := obj.(*networkingv1.NetworkPolicy)
	if !ok {
		return Description{}, false
	}
	attrs := map[string]inventory.Value{
		AttrSelector: inventory.Text(selectorText(&np.Spec.PodSelector)),
		AttrPolicyDigest: inventory.Text(
			digest([]byte(np.Spec.String()))),
		AttrDeniesEgress: inventory.Bool(deniesAllEgress(np)),
	}
	if rules, ok := policyRulesValue(np); ok {
		attrs[AttrPolicyRules] = rules
	}
	return Description{
		ID: objectID(KindNetworkPolicy, np), UID: string(np.UID),
		Attributes: attrs,
	}, true
}

// Diff implements Schema.
func (NetworkPolicySchema) Diff(old, new any) []inventory.FieldChange {
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
	if fields := policyChanges(before, after); len(fields) > 0 {
		return fields
	}
	return []inventory.FieldChange{{Path: "spec", Before: b, After: a}}
}

// AttrAttacher is the CSI driver that attaches a volume to a node, as a
// VolumeAttachment names it.
const AttrAttacher = "attacher"

// VolumeAttachmentSchema describes CSI volume attachments.
type VolumeAttachmentSchema struct{}

// Kind implements Schema.
func (VolumeAttachmentSchema) Kind() inventory.Kind {
	return KindVolumeAttachment
}

// RelationTypes implements Schema.
func (VolumeAttachmentSchema) RelationTypes() []inventory.RelationType {
	return []inventory.RelationType{inventory.References, inventory.RunsOn}
}

// Describe implements Schema.
func (VolumeAttachmentSchema) Describe(obj any) (Description, bool) {
	va, ok := obj.(*storagev1.VolumeAttachment)
	if !ok {
		return Description{}, false
	}
	attrs := map[string]inventory.Value{
		AttrAttached: inventory.Bool(va.Status.Attached),
		// A detach that never finishes is a deleting attachment: the
		// detector times it from when deletion was requested.
		AttrDeleting: inventory.Bool(va.DeletionTimestamp != nil),
	}
	if at := va.DeletionTimestamp; at != nil {
		attrs[AttrDeletingSince] = inventory.Time(at.Time)
	}
	if va.Spec.Attacher != "" {
		attrs[AttrAttacher] = inventory.Text(evidenceText(va.Spec.Attacher))
	}
	if err := va.Status.AttachError; err != nil {
		attrs[AttrAttachError] = inventory.Text(evidenceText(err.Message))
	}
	if err := va.Status.DetachError; err != nil {
		attrs[AttrDetachError] = inventory.Text(evidenceText(err.Message))
	}
	rel := relations{}
	rel.add(inventory.RunsOn,
		inventory.CoreID(KindNode, "", va.Spec.NodeName))
	if pv := va.Spec.Source.PersistentVolumeName; pv != nil {
		rel.add(inventory.References, inventory.CoreID(KindPV, "", *pv))
	}
	return Description{
		ID: objectID(KindVolumeAttachment, va), UID: string(va.UID),
		Attributes: attrs, Relations: rel,
	}, true
}

// Diff implements Schema.
func (VolumeAttachmentSchema) Diff(_, _ any) []inventory.FieldChange {
	return nil
}

// WebhookSchema describes admission webhook configurations. Each webhook
// that calls an in-cluster Service is served by it.
type WebhookSchema struct{ Mutating bool }

// Kind implements Schema.
func (w WebhookSchema) Kind() inventory.Kind {
	if w.Mutating {
		return KindMutatingWebhook
	}
	return KindValidatingHook
}

// RelationTypes implements Schema.
func (WebhookSchema) RelationTypes() []inventory.RelationType {
	return []inventory.RelationType{inventory.Serves}
}

// Describe implements Schema.
func (w WebhookSchema) Describe(obj any) (Description, bool) {
	var (
		name     string
		uid      string
		clients  []admissionv1.WebhookClientConfig
		policies []string
		hooks    []string
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
			hooks = append(hooks, hook.Name)
		}
	case *admissionv1.ValidatingWebhookConfiguration:
		if w.Mutating {
			return Description{}, false
		}
		name, uid = config.Name, string(config.UID)
		for _, hook := range config.Webhooks {
			clients = append(clients, hook.ClientConfig)
			policies = append(policies, failurePolicy(hook.FailurePolicy))
			hooks = append(hooks, hook.Name)
		}
	default:
		return Description{}, false
	}
	rel := relations{}
	for _, client := range clients {
		if svc := client.Service; svc != nil {
			rel.add(inventory.Serves, inventory.CoreID(
				KindService, svc.Namespace, svc.Name))
		}
	}
	return Description{
		ID: inventory.CoreID(w.Kind(), "", name), UID: uid,
		Attributes: map[string]inventory.Value{
			AttrFailurePolicy: inventory.Text(strings.Join(policies, ",")),
			AttrWebhookNames:  inventory.Text(strings.Join(hooks, ",")),
		},
		Relations: rel,
	}, true
}

// Diff implements Schema.
func (WebhookSchema) Diff(_, _ any) []inventory.FieldChange { return nil }

func failurePolicy(policy *admissionv1.FailurePolicyType) string {
	if policy == nil {
		return string(admissionv1.Fail)
	}
	return string(*policy)
}

// deniesAllEgress reports a policy that selects pods for egress and
// allows no egress at all.
func deniesAllEgress(np *networkingv1.NetworkPolicy) bool {
	for _, t := range np.Spec.PolicyTypes {
		if t == networkingv1.PolicyTypeEgress {
			return len(np.Spec.Egress) == 0
		}
	}
	return false
}

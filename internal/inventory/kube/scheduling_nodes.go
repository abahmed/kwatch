package kube

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Attributes the scheduling explainer reads, beside the CPU and memory
// ones. They are written only when there is something to say.
const (
	// AttrNodeLabels is a node's labels that no other attribute or
	// relation carries (not its name, zone or instance type), in the
	// format of AttrLabels. AttrNodeLabelsCut marks a node with too
	// many to keep: its labels are unknown.
	AttrNodeLabels    = "node.labels"
	AttrNodeLabelsCut = "node.labels.cut"
	// AttrAllocatable lists a node's other allocatable resources as
	// "name=value,...": pods, ephemeral-storage, extended resources.
	AttrAllocatable = "allocatable.extra"
	// AttrEphemeralReq and AttrExtendedReq are a container's requests
	// for ephemeral storage (bytes) and for the other resources, as
	// "name=value,...".
	AttrEphemeralReq = "ephemeral.request"
	AttrExtendedReq  = "extended.request"
	// AttrNodeTerms is on a PersistentVolume or StorageClass: the node
	// selector terms (JSON of [][]Requirement) a volume can attach by.
	AttrNodeTerms = "node.terms"
)

// maxNodeLabelBytes bounds the stored extra labels of one node.
const maxNodeLabelBytes = 4 << 10

// Labels the model keeps elsewhere on a node, so AttrNodeLabels skips
// them. NodeLabels puts them back.
var nodeKnownLabels = map[string]bool{
	corev1.LabelHostname:           true,
	corev1.LabelTopologyZone:       true,
	corev1.LabelInstanceTypeStable: true,
}

func setNodeScheduling(
	attrs map[string]inventory.Value, node *corev1.Node,
) {
	extra := map[string]string{}
	for key, value := range node.Labels {
		if !nodeKnownLabels[key] {
			extra[key] = value
		}
	}
	if text := labelText(extra); len(text) > maxNodeLabelBytes {
		attrs[AttrNodeLabelsCut] = inventory.Bool(true)
	} else if text != "" {
		attrs[AttrNodeLabels] = inventory.Text(text)
	}
	if text := extraResourcesText(node.Status.Allocatable); text != "" {
		attrs[AttrAllocatable] = inventory.Text(text)
	}
}

// extraResourcesText renders every resource except CPU and memory.
func extraResourcesText(list corev1.ResourceList) string {
	parts := make([]string, 0, len(list))
	for name, q := range list {
		if name == corev1.ResourceCPU || name == corev1.ResourceMemory {
			continue
		}
		parts = append(parts, string(name)+"="+strconv.FormatInt(q.Value(), 10))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func setContainerScheduling(
	attrs map[string]inventory.Value, c corev1.Container,
) {
	setQuantity(attrs, AttrEphemeralReq,
		c.Resources.Requests.StorageEphemeral())
	requests := corev1.ResourceList{}
	for name, q := range c.Resources.Limits {
		requests[name] = q
	}
	for name, q := range c.Resources.Requests {
		requests[name] = q
	}
	delete(requests, corev1.ResourceEphemeralStorage)
	if text := extraResourcesText(requests); text != "" {
		attrs[AttrExtendedReq] = inventory.Text(text)
	}
}

// ParseResourceList reads "name=value,..." into numbers.
func ParseResourceList(text string) map[string]float64 {
	out := map[string]float64{}
	for _, part := range strings.Split(text, ",") {
		name, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		if n, err := strconv.ParseFloat(value, 64); err == nil {
			out[name] = n
		}
	}
	return out
}

// Taint is one node taint.
type Taint struct{ Key, Value, Effect string }

// String renders "key=value:Effect", or "key:Effect" without a value.
func (t Taint) String() string {
	text := t.Key
	if t.Value != "" {
		text += "=" + t.Value
	}
	return text + ":" + t.Effect
}

// ParseTaints reads a node's AttrTaints text.
func ParseTaints(text string) []Taint {
	var out []Taint
	for _, part := range strings.Split(text, ",") {
		kv, effect, ok := strings.Cut(part, ":")
		if !ok {
			continue
		}
		key, value, _ := strings.Cut(kv, "=")
		out = append(out, Taint{Key: key, Value: value, Effect: effect})
	}
	return out
}

// Tolerates reports whether the toleration covers the taint.
func (t Toleration) Tolerates(taint Taint) bool {
	if t.Effect != "" && t.Effect != taint.Effect {
		return false
	}
	if t.Key != "" && t.Key != taint.Key {
		return false
	}
	if t.Operator == string(corev1.TolerationOpExists) {
		return true
	}
	return t.Value == taint.Value
}

// pvNodeTerms is the node selector of a PersistentVolume, as JSON.
func pvNodeTerms(pv *corev1.PersistentVolume) string {
	if pv.Spec.NodeAffinity == nil || pv.Spec.NodeAffinity.Required == nil {
		return ""
	}
	var terms [][]Requirement
	for _, term := range pv.Spec.NodeAffinity.Required.NodeSelectorTerms {
		var reqs []Requirement
		for _, r := range term.MatchExpressions {
			reqs = append(reqs, Requirement{Key: r.Key,
				Op: string(r.Operator), Values: r.Values})
		}
		terms = append(terms, reqs)
	}
	return encodeTerms(terms)
}

// classNodeTerms is the allowed topology of a StorageClass, as JSON.
func classNodeTerms(sc *storagev1.StorageClass) string {
	var terms [][]Requirement
	for _, term := range sc.AllowedTopologies {
		var reqs []Requirement
		for _, r := range term.MatchLabelExpressions {
			reqs = append(reqs, Requirement{Key: r.Key, Op: "In",
				Values: r.Values})
		}
		terms = append(terms, reqs)
	}
	return encodeTerms(terms)
}

func encodeTerms(terms [][]Requirement) string {
	if len(terms) == 0 {
		return ""
	}
	data, err := json.Marshal(terms)
	if err != nil {
		return ""
	}
	return string(data)
}

// ParseNodeTerms reads an AttrNodeTerms value.
func ParseNodeTerms(e inventory.Entity) [][]Requirement {
	var terms [][]Requirement
	if v, ok := e.Attribute(AttrNodeTerms); ok {
		_ = json.Unmarshal([]byte(v.Value.AsText()), &terms)
	}
	return terms
}

// NodeLabels rebuilds a node's labels from what the model keeps of
// them: the zone, which the model stores as a relation, comes in zone.
// The second result is false when the node had too many labels to keep.
func NodeLabels(node inventory.Entity, zone string) (map[string]string, bool) {
	labels := ParseLabels(textOf(node, AttrNodeLabels))
	labels[corev1.LabelHostname] = node.ID.Name
	if zone != "" {
		labels[corev1.LabelTopologyZone] = zone
	}
	if it := textOf(node, AttrInstanceType); it != "" {
		labels[corev1.LabelInstanceTypeStable] = it
	}
	cut, ok := node.Attribute(AttrNodeLabelsCut)
	isCut, _ := cut.Value.AsBool()
	return labels, !(ok && isCut)
}

// ParseLabels reads labels written by labelText ("k=v,k=v").
func ParseLabels(text string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(text, ",") {
		if key, value, ok := strings.Cut(part, "="); ok {
			out[key] = value
		}
	}
	return out
}

func textOf(e inventory.Entity, name string) string {
	if v, ok := e.Attribute(name); ok {
		return v.Value.AsText()
	}
	return ""
}

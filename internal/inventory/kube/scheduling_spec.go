package kube

import (
	"encoding/json"
	"slices"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// AttrSchedulingSpec is on a pod no node holds yet. It is the part of the
// pod spec the scheduler filters nodes by, as JSON of SchedulingSpec. It
// is absent when the pod has no such constraint.
const AttrSchedulingSpec = "scheduling.spec"

// maxSchedulingSpecBytes bounds the stored spec. A bigger one is stored
// as TooBig, so a reader knows it cannot judge the pod.
const maxSchedulingSpecBytes = 8 << 10

// Requirement is one label requirement, from a node selector term or a
// label selector: Op is In, NotIn, Exists, DoesNotExist, Gt or Lt.
type Requirement struct {
	Key    string   `json:"k"`
	Op     string   `json:"o"`
	Values []string `json:"v,omitempty"`
}

// Toleration is a pod toleration.
type Toleration struct {
	Key      string `json:"k,omitempty"`
	Operator string `json:"o,omitempty"`
	Value    string `json:"v,omitempty"`
	Effect   string `json:"e,omitempty"`
}

// PodTerm is one required pod (anti-)affinity term. Any is set when the
// term picks namespaces by a selector, which is not evaluated: every
// namespace is then searched.
type PodTerm struct {
	Selector    []Requirement `json:"s,omitempty"`
	Namespaces  []string      `json:"n,omitempty"`
	Any         bool          `json:"a,omitempty"`
	TopologyKey string        `json:"t"`
}

// Spread is a topology spread constraint that blocks scheduling.
type Spread struct {
	TopologyKey string        `json:"t"`
	MaxSkew     int           `json:"m"`
	MinDomains  int           `json:"d,omitempty"`
	Selector    []Requirement `json:"s,omitempty"`
}

// SchedulingSpec is what a pod asks of the node it runs on.
type SchedulingSpec struct {
	TooBig      bool              `json:"big,omitempty"`
	Selector    map[string]string `json:"sel,omitempty"`
	Tolerations []Toleration      `json:"tol,omitempty"`
	// NodeTerms are the required node affinity terms: a node must
	// satisfy every requirement of at least one term.
	NodeTerms [][]Requirement `json:"na,omitempty"`
	Affinity  []PodTerm       `json:"pa,omitempty"`
	Anti      []PodTerm       `json:"aa,omitempty"`
	Spread    []Spread        `json:"ts,omitempty"`
}

// empty reports a pod with no constraint of this kind.
func (s SchedulingSpec) empty() bool {
	return len(s.Selector) == 0 && len(s.Tolerations) == 0 &&
		len(s.NodeTerms) == 0 && len(s.Affinity) == 0 &&
		len(s.Anti) == 0 && len(s.Spread) == 0
}

// ParseSchedulingSpec decodes a pod's AttrSchedulingSpec. A missing or
// unreadable value is the empty spec: the pod asks nothing special.
func ParseSchedulingSpec(e inventory.Entity) SchedulingSpec {
	var spec SchedulingSpec
	if v, ok := e.Attribute(AttrSchedulingSpec); ok {
		_ = json.Unmarshal([]byte(v.Value.AsText()), &spec)
	}
	return spec
}

// setSchedulingSpec records the scheduling constraints of a pod that no
// node holds yet; a placed pod no longer needs them.
func setSchedulingSpec(attrs map[string]inventory.Value, pod *corev1.Pod) {
	if pod.Spec.NodeName != "" {
		return
	}
	spec := buildSchedulingSpec(pod)
	if spec.empty() {
		return
	}
	data, err := json.Marshal(spec)
	if err != nil || len(data) > maxSchedulingSpecBytes {
		data = []byte(`{"big":true}`)
	}
	attrs[AttrSchedulingSpec] = inventory.Text(string(data))
}

func buildSchedulingSpec(pod *corev1.Pod) SchedulingSpec {
	spec := SchedulingSpec{Selector: pod.Spec.NodeSelector}
	for _, t := range pod.Spec.Tolerations {
		spec.Tolerations = append(spec.Tolerations, Toleration{
			Key: t.Key, Operator: string(t.Operator), Value: t.Value,
			Effect: string(t.Effect),
		})
	}
	spec.NodeTerms = nodeTerms(pod.Spec.Affinity)
	if a := pod.Spec.Affinity; a != nil {
		if a.PodAffinity != nil {
			spec.Affinity = podTerms(a.PodAffinity.
				RequiredDuringSchedulingIgnoredDuringExecution)
		}
		if a.PodAntiAffinity != nil {
			spec.Anti = podTerms(a.PodAntiAffinity.
				RequiredDuringSchedulingIgnoredDuringExecution)
		}
	}
	for _, c := range pod.Spec.TopologySpreadConstraints {
		if c.WhenUnsatisfiable != corev1.DoNotSchedule {
			continue
		}
		spread := Spread{TopologyKey: c.TopologyKey,
			MaxSkew: int(c.MaxSkew), Selector: selectorTerms(c.LabelSelector)}
		if c.MinDomains != nil {
			spread.MinDomains = int(*c.MinDomains)
		}
		spec.Spread = append(spec.Spread, spread)
	}
	return spec
}

func nodeTerms(a *corev1.Affinity) [][]Requirement {
	if a == nil || a.NodeAffinity == nil ||
		a.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		return nil
	}
	var out [][]Requirement
	required := a.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	for _, term := range required.NodeSelectorTerms {
		var reqs []Requirement
		for _, r := range term.MatchExpressions {
			reqs = append(reqs, Requirement{Key: r.Key,
				Op: string(r.Operator), Values: r.Values})
		}
		out = append(out, reqs)
	}
	return out
}

func podTerms(terms []corev1.PodAffinityTerm) []PodTerm {
	var out []PodTerm
	for _, t := range terms {
		out = append(out, PodTerm{
			Selector: selectorTerms(t.LabelSelector), Namespaces: t.Namespaces,
			Any:         t.NamespaceSelector != nil,
			TopologyKey: t.TopologyKey,
		})
	}
	return out
}

// selectorTerms flattens a label selector into requirements.
func selectorTerms(sel *metav1.LabelSelector) []Requirement {
	if sel == nil {
		return nil
	}
	var out []Requirement
	for _, key := range sortedKeys(sel.MatchLabels) {
		out = append(out, Requirement{Key: key, Op: "In",
			Values: []string{sel.MatchLabels[key]}})
	}
	for _, r := range sel.MatchExpressions {
		out = append(out, Requirement{Key: r.Key, Op: string(r.Operator),
			Values: r.Values})
	}
	return out
}

// Matches reports whether labels satisfy the requirement.
func (r Requirement) Matches(labels map[string]string) bool {
	value, has := labels[r.Key]
	switch r.Op {
	case "In":
		return has && slices.Contains(r.Values, value)
	case "NotIn":
		return !has || !slices.Contains(r.Values, value)
	case "Exists":
		return has
	case "DoesNotExist":
		return !has
	case "Gt", "Lt":
		return has && compareInts(value, r.Values, r.Op == "Gt")
	}
	return false
}

// MatchesAll reports whether labels satisfy every requirement. An empty
// list matches everything.
func MatchesAll(reqs []Requirement, labels map[string]string) bool {
	for _, r := range reqs {
		if !r.Matches(labels) {
			return false
		}
	}
	return true
}

func compareInts(value string, bound []string, greater bool) bool {
	if len(bound) != 1 {
		return false
	}
	have, err1 := strconv.ParseInt(value, 10, 64)
	want, err2 := strconv.ParseInt(bound[0], 10, 64)
	if err1 != nil || err2 != nil {
		return false
	}
	if greater {
		return have > want
	}
	return have < want
}

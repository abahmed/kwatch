package kube

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

func describePod(t *testing.T, pod *corev1.Pod) Description {
	t.Helper()
	d, ok := PodSchema{}.Describe(pod)
	require.True(t, ok)
	return d
}

func TestSchedulingSpecAbsentForPlainPod(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p"}}
	_, has := describePod(t, pod).Attributes[AttrSchedulingSpec]
	assert.False(t, has)
}

func TestSchedulingSpecAbsentOnceScheduled(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p"},
		Spec: corev1.PodSpec{NodeName: "n1",
			NodeSelector: map[string]string{"a": "b"}}}
	_, has := describePod(t, pod).Attributes[AttrSchedulingSpec]
	assert.False(t, has)
}

func TestSchedulingSpecRoundTrip(t *testing.T) {
	min := int32(2)
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p"},
		Spec: corev1.PodSpec{
			NodeSelector: map[string]string{"disk": "ssd"},
			Tolerations: []corev1.Toleration{{Key: "gpu",
				Operator: corev1.TolerationOpExists}},
			Affinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{{
							MatchExpressions: []corev1.NodeSelectorRequirement{{
								Key: "tier", Operator: corev1.NodeSelectorOpIn,
								Values: []string{"gold"}}}}}}},
				PodAntiAffinity: &corev1.PodAntiAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "web"}},
						TopologyKey: "kubernetes.io/hostname"}}},
			},
			TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
				{MaxSkew: 1, TopologyKey: "zone", MinDomains: &min,
					WhenUnsatisfiable: corev1.DoNotSchedule},
				{MaxSkew: 1, TopologyKey: "soft",
					WhenUnsatisfiable: corev1.ScheduleAnyway},
			}}}
	entity := entityOf(describePod(t, pod).Attributes)
	spec := ParseSchedulingSpec(entity)
	assert.Equal(t, map[string]string{"disk": "ssd"}, spec.Selector)
	assert.Equal(t, []Toleration{{Key: "gpu", Operator: "Exists"}},
		spec.Tolerations)
	assert.Equal(t, [][]Requirement{{{Key: "tier", Op: "In",
		Values: []string{"gold"}}}}, spec.NodeTerms)
	require.Len(t, spec.Anti, 1)
	assert.Equal(t, "kubernetes.io/hostname", spec.Anti[0].TopologyKey)
	assert.Equal(t, []Requirement{{Key: "app", Op: "In",
		Values: []string{"web"}}}, spec.Anti[0].Selector)
	assert.Equal(t, []Spread{{TopologyKey: "zone", MaxSkew: 1,
		MinDomains: 2}}, spec.Spread)
}

func TestSchedulingSpecTooBigIsMarked(t *testing.T) {
	selector := map[string]string{}
	for i := 0; i < 400; i++ {
		selector["label-key-number-"+string(rune('a'+i%26))+
			string(rune('a'+i/26))] = "value-for-the-label"
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p"},
		Spec: corev1.PodSpec{NodeSelector: selector}}
	entity := entityOf(describePod(t, pod).Attributes)
	assert.True(t, ParseSchedulingSpec(entity).TooBig)
}

func TestRequirementMatches(t *testing.T) {
	labels := map[string]string{"a": "1", "n": "5"}
	for _, tc := range []struct {
		r    Requirement
		want bool
	}{
		{Requirement{"a", "In", []string{"1", "2"}}, true},
		{Requirement{"a", "In", []string{"2"}}, false},
		{Requirement{"a", "NotIn", []string{"2"}}, true},
		{Requirement{"b", "NotIn", []string{"2"}}, true},
		{Requirement{"a", "Exists", nil}, true},
		{Requirement{"b", "DoesNotExist", nil}, true},
		{Requirement{"n", "Gt", []string{"3"}}, true},
		{Requirement{"n", "Lt", []string{"3"}}, false},
		{Requirement{"a", "Gt", []string{"x"}}, false},
		{Requirement{"a", "Weird", nil}, false},
	} {
		assert.Equal(t, tc.want, tc.r.Matches(labels), tc.r)
	}
	assert.True(t, MatchesAll(nil, labels))
}

func TestTolerates(t *testing.T) {
	taint := Taint{Key: "gpu", Value: "true", Effect: "NoSchedule"}
	for _, tc := range []struct {
		t    Toleration
		want bool
	}{
		{Toleration{Key: "gpu", Operator: "Exists"}, true},
		{Toleration{Key: "gpu", Operator: "Equal", Value: "true"}, true},
		{Toleration{Key: "gpu", Value: "false"}, false},
		{Toleration{Operator: "Exists"}, true},
		{Toleration{Key: "gpu", Operator: "Exists", Effect: "NoExecute"},
			false},
		{Toleration{Key: "other", Operator: "Exists"}, false},
	} {
		assert.Equal(t, tc.want, tc.t.Tolerates(taint), tc.t)
	}
}

func TestParseTaintsRoundTrip(t *testing.T) {
	taints := ParseTaints("a=b:NoSchedule,c:NoExecute")
	assert.Equal(t, []Taint{{"a", "b", "NoSchedule"},
		{"c", "", "NoExecute"}}, taints)
	assert.Equal(t, "a=b:NoSchedule", taints[0].String())
	assert.Equal(t, "c:NoExecute", taints[1].String())
}

func TestNodeSchedulingAttributes(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1",
		Labels: map[string]string{
			corev1.LabelHostname: "n1", "karpenter.sh/nodepool": "gpu",
			"disk": "ssd"}},
		Status: corev1.NodeStatus{Allocatable: corev1.ResourceList{
			corev1.ResourceCPU:  resource.MustParse("4"),
			corev1.ResourcePods: resource.MustParse("110"),
			"nvidia.com/gpu":    resource.MustParse("2"),
		}}}
	d, ok := NodeSchema{}.Describe(node)
	require.True(t, ok)
	entity := entityOf(d.Attributes)
	entity.ID = d.ID
	labels, known := NodeLabels(entity, "eu-1a")
	assert.True(t, known)
	assert.Equal(t, "ssd", labels["disk"])
	assert.Equal(t, "gpu", labels["karpenter.sh/nodepool"])
	assert.Equal(t, "n1", labels[corev1.LabelHostname])
	assert.Equal(t, "eu-1a", labels[corev1.LabelTopologyZone])
	assert.Equal(t, map[string]float64{"pods": 110, "nvidia.com/gpu": 2},
		ParseResourceList(d.Attributes[AttrAllocatable].AsText()))
}

func TestNodeWithPlainLabelsAddsNoAttributes(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1",
		Labels: map[string]string{corev1.LabelHostname: "n1"}}}
	d, _ := NodeSchema{}.Describe(node)
	_, labels := d.Attributes[AttrNodeLabels]
	_, extra := d.Attributes[AttrAllocatable]
	assert.False(t, labels || extra)
}

func TestContainerSchedulingAttributes(t *testing.T) {
	c := corev1.Container{Name: "app", Resources: corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceEphemeralStorage: resource.MustParse("2Gi")},
		Limits: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("1")},
	}}
	attrs := containerSpecAttributes(c, false)
	v, _ := attrs[AttrEphemeralReq].AsNumber()
	assert.Equal(t, float64(2<<30), v)
	assert.Equal(t, "nvidia.com/gpu=1", attrs[AttrExtendedReq].AsText())
}

func TestVolumeAndClassNodeTerms(t *testing.T) {
	pv := &corev1.PersistentVolume{Spec: corev1.PersistentVolumeSpec{
		NodeAffinity: &corev1.VolumeNodeAffinity{
			Required: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{{
					MatchExpressions: []corev1.NodeSelectorRequirement{{
						Key: "zone", Operator: corev1.NodeSelectorOpIn,
						Values: []string{"a"}}}}}}}}}
	d, _ := PVSchema{}.Describe(pv)
	terms := ParseNodeTerms(entityOf(d.Attributes))
	assert.Equal(t, [][]Requirement{{{"zone", "In", []string{"a"}}}}, terms)
	sc := &storagev1.StorageClass{}
	sc.AllowedTopologies = []corev1.TopologySelectorTerm{{
		MatchLabelExpressions: []corev1.TopologySelectorLabelRequirement{{
			Key: "zone", Values: []string{"b"}}}}}
	d, _ = StorageClassSchema{}.Describe(sc)
	terms = ParseNodeTerms(entityOf(d.Attributes))
	assert.Equal(t, [][]Requirement{{{"zone", "In", []string{"b"}}}}, terms)
}

func TestEventNoteKeepsAutoscalerNormalEvents(t *testing.T) {
	for _, tc := range []struct {
		kind, reason string
		want         bool
	}{
		{corev1.EventTypeNormal, ReasonTriggeredScaleUp, true},
		{corev1.EventTypeNormal, ReasonNotTriggerScaleUp, true},
		{corev1.EventTypeNormal, ReasonNominated, true},
		{corev1.EventTypeNormal, "Pulled", false},
		{corev1.EventTypeWarning, "Pulled", true},
	} {
		ev := &corev1.Event{Type: tc.kind, Reason: tc.reason,
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "p"}}
		obs, ok := EventNote(ev, time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
		assert.Equal(t, tc.want, ok, tc.reason)
		if ok {
			assert.Equal(t, tc.kind == corev1.EventTypeWarning,
				obs.Note.Warning)
		}
	}
}

// entityOf wraps described attributes the way the model stores them.
func entityOf(attrs map[string]inventory.Value) inventory.Entity {
	stored := make(map[string]inventory.Attribute, len(attrs))
	for name, value := range attrs {
		stored[name] = inventory.Attribute{Value: value}
	}
	return inventory.Entity{Attributes: stored}
}

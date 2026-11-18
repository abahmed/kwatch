package incident

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// archSplit puts deployment api on two arm64 nodes (three pods, all
// failing when armFail) and two amd64 nodes (four healthy pods). The
// failing pods are returned.
func archSplit(s *shapeModel) []inventory.EntityID {
	deploy := entity(kube.KindDeployment, "api")
	rs := s.replicaSet("api-1", deploy)
	arm := []inventory.EntityID{s.node("a1", "arm64", "t4g"),
		s.node("a2", "arm64", "t4g")}
	amd := []inventory.EntityID{s.node("b1", "amd64", "m5"),
		s.node("b2", "amd64", "m5")}
	var failing []inventory.EntityID
	for i, name := range []string{"api-a", "api-b", "api-c"} {
		failing = append(failing, s.pod(name, rs, arm[i%2], false))
	}
	for i, name := range []string{"api-d", "api-e", "api-f", "api-g"} {
		s.pod(name, rs, amd[i%2], true)
	}
	return failing
}

func TestReplicaSplitNamesWhatOnlyTheFailingPodsShare(t *testing.T) {
	s := newShapeModel(t)
	failing := archSplit(s)

	split := ReplicaSplitOf(s.m, crashIncident(failing[0], failing...))

	require.NotNil(t, split)
	assert.Equal(t, "api", split.Workload.Name)
	assert.Equal(t, 3, split.Failing)
	assert.Equal(t, 4, split.Healthy)
	assert.Equal(t, []string{
		"Only the 3 pods on arm64 nodes fail; the 4 pods on amd64 " +
			"nodes are healthy.",
		"Only the 3 pods on t4g nodes fail; the 4 pods on m5 nodes " +
			"are healthy.",
	}, split.Lines, "at most two differences, the architecture first")
}

func TestReplicaSplitSaysNothingWhenTheSplitIsMixed(t *testing.T) {
	s := newShapeModel(t)
	failing := archSplit(s)
	// One more failing pod on an amd64 node: no clean split.
	rs := entity(kube.KindReplicaSet, "api-1")
	extra := s.pod("api-h", rs, inventory.CoreID(kube.KindNode, "", "b1"),
		false)

	p := crashIncident(failing[0], append(failing, extra)...)

	assert.Nil(t, ReplicaSplitOf(s.m, p))
}

func TestReplicaSplitNeedsTwoFailingAndOneHealthy(t *testing.T) {
	s := newShapeModel(t)
	failing := archSplit(s)
	assert.Nil(t, ReplicaSplitOf(s.m, crashIncident(failing[0], failing[0])),
		"one failing pod differs from the rest in many ways")

	s2 := newShapeModel(t)
	n1 := s2.node("n1", "amd64", "m5")
	rs := s2.replicaSet("api-1", entity(kube.KindDeployment, "api"))
	a, b := s2.pod("api-a", rs, n1, false), s2.pod("api-b", rs, n1, false)
	assert.Nil(t, ReplicaSplitOf(s2.m, crashIncident(a, a, b)),
		"no healthy pod to compare with")
}

func TestReplicaSplitNamesTheNewReplicaSet(t *testing.T) {
	s := newShapeModel(t)
	deploy := entity(kube.KindDeployment, "api")
	old := s.replicaSet("api-old", deploy)
	fresh := s.replicaSet("api-new", deploy)
	n1, n2 := s.node("n1", "amd64", "m5"), s.node("n2", "amd64", "m5")
	a, b := s.pod("api-a", fresh, n1, false),
		s.pod("api-b", fresh, n2, false)
	s.pod("api-c", old, n1, true)
	s.pod("api-d", old, n2, true)

	split := ReplicaSplitOf(s.m, crashIncident(a, a, b))

	require.NotNil(t, split)
	assert.Equal(t, []string{"Only the 2 pods of ReplicaSet api-new " +
		"fail; the 2 pods of ReplicaSet api-old are healthy."}, split.Lines)
}

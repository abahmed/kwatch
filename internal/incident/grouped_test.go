package incident

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// twoWorkloads puts deployments a (two pods) and b (one pod) on node
// n1 and returns the node and the three pods.
func twoWorkloads(s *shapeModel) (inventory.EntityID, []inventory.EntityID) {
	n1 := s.node("n1", "amd64", "m5.large")
	a := s.replicaSet("a-1", entity(kube.KindDeployment, "a"))
	b := s.replicaSet("b-1", entity(kube.KindDeployment, "b"))
	return n1, []inventory.EntityID{s.pod("a-1-x", a, n1, false),
		s.pod("a-1-y", a, n1, false), s.pod("b-1-x", b, n1, false)}
}

func TestGroupingNamesTheNodeTheyShare(t *testing.T) {
	s := newShapeModel(t)
	n1, pods := twoWorkloads(s)
	s.set(n1, map[string]inventory.Value{kube.AttrReady: inventory.Bool(false)})

	g := GroupingOf(s.m, crashIncident(n1, pods...))

	require.NotNil(t, g)
	assert.Equal(t, "Grouped because all 3 pods run on node n1, "+
		"which is not ready.", g.Text)
	assert.Equal(t, GroupedNode, g.Basis)
	require.Len(t, g.Groups, 2)
	assert.Equal(t, "a", g.Groups[0].Owner.Name, "the largest first")
	assert.Len(t, g.Groups[0].Members, 2)
	assert.Equal(t, "b", g.Groups[1].Owner.Name)
}

func TestGroupingRestsOnTheCause(t *testing.T) {
	s := newShapeModel(t)
	_, pods := twoWorkloads(s)
	db := entity(kube.KindService, "db")
	image := inventory.CoreID(kube.KindImage, "", "payments:1.5")
	cases := []struct {
		name  string
		cause rootcause.CauseRecord
		basis string
		want  string
	}{
		{"shared error line", rootcause.CauseRecord{
			Root:        inventory.CoreID(explain.KindFailureSignature, "", "x"),
			SharedError: "cannot reach db.example.com:5432."}, GroupedError,
			"Grouped because 2 workloads log the same error: " +
				"cannot reach db.example.com:5432."},
		{"shared dependency", rootcause.CauseRecord{Root: db},
			GroupedDependency, "Grouped because both workloads call " +
				"service db."},
		{"shared release", rootcause.CauseRecord{Root: image,
			Change: &inventory.Change{Entity: image}}, GroupedImage,
			"Grouped because both workloads were changed to image " +
				"payments:1.5."},
		{"another cause", rootcause.CauseRecord{
			Root: entity(kube.KindLease, "lock")}, GroupedCause,
			"Grouped because kwatch traced both workloads to lease lock."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := crashIncident(c.cause.Root, pods...)
			p.Cause = &c.cause
			g := GroupingOf(s.m, p)
			require.NotNil(t, g)
			assert.Equal(t, c.want, g.Text)
			assert.Equal(t, c.basis, g.Basis)
		})
	}
}

func TestGroupingSaysNothingForOneWorkload(t *testing.T) {
	s := newShapeModel(t)
	n1, pods := twoWorkloads(s)
	p := crashIncident(n1, pods[:2]...)
	assert.Nil(t, GroupingOf(s.m, p), "both pods are of deployment a")

	deploy := entity(kube.KindDeployment, "a")
	p = crashIncident(deploy, pods...)
	assert.Nil(t, GroupingOf(s.m, p),
		"two workloads but no cause: nothing names what they share")
}

func TestGroupingBoundsTheMembers(t *testing.T) {
	s := newShapeModel(t)
	n1 := s.node("n1", "amd64", "m5.large")
	var pods []inventory.EntityID
	for i := range MaxGroups + 2 {
		rs := s.replicaSet(string(rune('a'+i))+"-1",
			entity(kube.KindDeployment, string(rune('a'+i))))
		for j := range MaxGroupMembers + 1 {
			pods = append(pods, s.pod(rs.Name+string(rune('p'+j)), rs,
				n1, false))
		}
	}

	g := GroupingOf(s.m, crashIncident(n1, pods...))

	require.NotNil(t, g)
	assert.Len(t, g.Groups, MaxGroups)
	assert.Equal(t, 2, g.More)
	assert.Len(t, g.Groups[0].Members, MaxGroupMembers)
	assert.Equal(t, 1, g.Groups[0].More)
}

// A long shared line is cut after a whole word, and members of one
// kind are named by it.
func TestGroupingWording(t *testing.T) {
	long := "connection refused " + strings.Repeat("word ", 40)
	line := sharedLine(long)
	assert.True(t, strings.HasSuffix(line, "word …"), line)
	assert.LessOrEqual(t, len([]rune(line)), maxGroupedError+2)
	assert.Equal(t, "connection refused", sharedLine("connection refused."))

	zone := inventory.CoreID(kube.KindZone, "", "z1")
	p := crashIncident(zone, inventory.CoreID(kube.KindNode, "", "n1"),
		inventory.CoreID(kube.KindNode, "", "n2"))
	g := GroupingOf(nil, p)
	require.NotNil(t, g)
	assert.Equal(t, "Grouped because both nodes run in zone z1.", g.Text)
	assert.Equal(t, GroupedPlace, g.Basis)
}

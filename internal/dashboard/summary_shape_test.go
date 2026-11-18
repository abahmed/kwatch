package dashboard

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// shapePod adds a pod of rs on node, ready or not.
func (tm *testModel) shapePod(
	name string, rs, node inventory.EntityID, ready bool,
) inventory.EntityID {
	pod := inventory.CoreID(kube.KindPod, "shop", name)
	tm.observe(pod, map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(ready)})
	tm.relate(pod, inventory.OwnedBy, rs)
	tm.relate(pod, inventory.RunsOn, node)
	return pod
}

// shapeNode adds a node with an architecture.
func (tm *testModel) shapeNode(name, arch string) inventory.EntityID {
	node := inventory.CoreID(kube.KindNode, "", name)
	tm.observe(node, map[string]inventory.Value{
		kube.AttrReady:      inventory.Bool(true),
		kube.AttrNodeLabels: inventory.Text("kubernetes.io/arch=" + arch)})
	return node
}

// shapeFixture is deployment api in shop: two pods failing on arm64
// nodes, two healthy on amd64 nodes, behind Service api with 2 of 4
// endpoints ready, which shop.example.com routes to.
func shapeFixture(t *testing.T) (*testModel, incident.Incident) {
	tm := newTestModel(t)
	deploy := inventory.CoreID(kube.KindDeployment, "shop", "api")
	rs := inventory.CoreID(kube.KindReplicaSet, "shop", "api-1")
	tm.observe(deploy, nil)
	tm.observe(rs, nil)
	tm.relate(rs, inventory.OwnedBy, deploy)
	arm, amd := tm.shapeNode("a1", "arm64"), tm.shapeNode("b1", "amd64")
	bad := []inventory.EntityID{tm.shapePod("api-a", rs, arm, false),
		tm.shapePod("api-b", rs, arm, false)}
	tm.shapePod("api-c", rs, amd, true)
	tm.shapePod("api-d", rs, amd, true)
	svc := inventory.CoreID(kube.KindService, "shop", "api")
	tm.observe(svc, nil)
	tm.relate(svc, inventory.Selects, bad...)
	tm.slice(svc, "api-x", 4, 2)
	tm.observe(inventory.CoreID(kube.KindIngress, "shop", "web"),
		map[string]inventory.Value{kube.AttrPublicAddresses: inventory.Text(
			"https\tshop.example.com\t/\tapi\t80")})
	p := failingAt("t", deploy, testEpoch.Add(-time.Hour))
	for _, pod := range bad {
		f := detection.Finding{Entity: pod, Reason: "CrashLoopBackOff",
			Since: p.Opened}
		p.Members[f.Key()] = f
	}
	return tm, p
}

func TestSummaryShowsWhetherUsersAreAffected(t *testing.T) {
	tm, p := shapeFixture(t)

	d := detailOf(t, contextServer(tm.m, false, p), "t")

	require.NotNil(t, d.Summary)
	require.NotNil(t, d.Summary.Users)
	assert.Equal(t, SummaryUsers{State: incident.UsersServed,
		Text: "Users are still served: route shop.example.com → api " +
			"has 2 of 4 ready endpoints.", Route: "shop.example.com",
		Service: "api", Ready: 2, Total: 4}, *d.Summary.Users)
}

func TestSummaryComparesHealthyAndFailingReplicas(t *testing.T) {
	tm, p := shapeFixture(t)

	d := detailOf(t, contextServer(tm.m, false, p), "t")

	require.NotNil(t, d.Summary)
	assert.Equal(t, []string{
		"Only the 2 pods on arm64 nodes fail; the 2 pods on amd64 " +
			"nodes are healthy.",
		"Only the 2 pods on node a1 fail; the 2 pods on node b1 are " +
			"healthy.",
	}, d.Summary.Replicas)
	assert.Nil(t, d.Summary.Grouped, "one workload is not a group")
}

func TestSummaryShapeSpeaksOfNowOnly(t *testing.T) {
	tm, p := shapeFixture(t)
	p.State, p.Resolved = incident.Resolved, testEpoch

	d := detailOf(t, contextServer(tm.m, false, p), "t")

	require.NotNil(t, d.Summary)
	assert.Nil(t, d.Summary.Users)
	assert.Empty(t, d.Summary.Replicas)
}

func TestSummaryListsTheMembersByGroup(t *testing.T) {
	tm, _ := shapeFixture(t)
	node := tm.shapeNode("n9", "amd64")
	tm.observe(node, map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(false)})
	p := failingAt("n", node, testEpoch.Add(-time.Hour))
	for _, name := range []string{"x", "y"} {
		rs := inventory.CoreID(kube.KindReplicaSet, "shop", name+"-1")
		tm.observe(rs, nil)
		tm.relate(rs, inventory.OwnedBy,
			inventory.CoreID(kube.KindDeployment, "shop", name))
		pod := tm.shapePod(name+"-a", rs, node, false)
		f := detection.Finding{Entity: pod, Reason: "CrashLoopBackOff"}
		p.Members[f.Key()] = f
	}

	d := detailOf(t, contextServer(tm.m, false, p), "n")

	require.NotNil(t, d.Summary)
	require.NotNil(t, d.Summary.Grouped)
	g := d.Summary.Grouped
	assert.Equal(t, "Grouped because both pods run on node n9, which "+
		"is not ready.", g.Text)
	assert.Equal(t, incident.GroupedNode, g.Basis)
	require.Len(t, g.Groups, 2)
	assert.Equal(t, SummaryObject{Kind: "deployment", Namespace: "shop",
		Name: "x"}, g.Groups[0].Owner)
	assert.Equal(t, []SummaryObject{{Kind: "pod", Namespace: "shop",
		Name: "x-a"}}, g.Groups[0].Members)
}

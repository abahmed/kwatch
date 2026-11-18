package dashboard

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// holderIncident is an incident rooted at root whose members are the
// given findings, each failing in mode.
func holderIncident(root inventory.EntityID, mode detection.Mode,
	members ...inventory.EntityID) incident.Incident {
	p := summaryIncident()
	p.Root, p.Mode, p.Headline = root, mode, "something is failing"
	p.Cause = &rootcause.CauseRecord{Root: root, Score: 0.9}
	p.Members = map[detection.Key]detection.Finding{}
	for _, id := range members {
		p.Members[detection.Key{Entity: id, Reason: string(mode)}] =
			detection.Finding{Entity: id, Mode: mode}
	}
	return p
}

func objID(kind, namespace, name string) inventory.EntityID {
	return inventory.EntityID{Kind: inventory.Kind(kind),
		Namespace: namespace, Name: name}
}

// The subject of "What's wrong?" is what has the symptom: a member,
// when the root is an object of a kind that cannot have it.
func TestSymptomIsNeverPutOnAKindThatCannotHaveIt(t *testing.T) {
	cases := []struct {
		name   string
		root   inventory.EntityID
		mode   detection.Mode
		member inventory.EntityID
		want   string
	}{
		{"secret root, pod member", objID("secret", "shop", "db-creds"),
			detection.ModeMissingSecret, objID("pod", "shop", "web-5c57-x"),
			"Pod web-5c57-x in shop couldn't find a Secret it needs."},
		{"aggregated API root, autoscaler member",
			objID("apiservice", "", "v1.metrics.example.io"),
			detection.ModeScalingNoMetrics,
			objID("horizontalpodautoscaler", "shop", "web"),
			"HorizontalPodAutoscaler web in shop couldn't scale " +
				"automatically."},
		{"service root, container member", objID("service", "shop", "lookup"),
			detection.ModeError, objID("container", "shop", "web-7d9f-a/web"),
			"Container web-7d9f-a/web in shop exited with an error."},
		{"node root, service member", objID("node", "", "node-a"),
			detection.ModeNoEndpoints, objID("service", "shop", "web"),
			"Service web in shop had no ready pods to send traffic to."},
	}
	for _, c := range cases {
		p := holderIncident(c.root, c.mode, c.member)
		p.State = incident.Resolved
		d := toDetail(p)

		got := whatOf(&d, p)

		assert.Equal(t, c.want, got, c.name)
		assert.NotContains(t, got, rootWords(c.root), c.name)
	}
}

// With no member that can have it, the symptom is not said at all: the
// incident's own title or a member's finding stands instead.
func TestSymptomWithoutAHolderFallsBackToTheFinding(t *testing.T) {
	p := holderIncident(objID("node", "", "node-a"), detection.ModeError)
	p.Members[detection.Key{Entity: p.Root, Reason: "x"}] =
		detection.Finding{Entity: p.Root, Summary: "Node has run out " +
			"of pod IP addresses"}
	d := toDetail(p)

	got := whatOf(&d, p)

	assert.NotContains(t, got, "error")
	assert.Equal(t, "Node has run out of pod IP addresses.", got)
}

// A readiness check is a workload's: a node root does not fail one, its
// pod does.
func TestFailingProbeIsSaidOfTheWorkloadNotTheNode(t *testing.T) {
	pod := objID("pod", "ops", "probe-ctl-69b4-d4b2c")
	p := holderIncident(objID("node", "", "node-a"), detection.ModeNotReady,
		pod)
	d := toDetail(p)
	d.Probes = []ProbeRow{{Probe: "readiness"}}

	assert.Equal(t, "Pod probe-ctl-69b4-d4b2c in ops is failing its "+
		"readiness check, so it gets no traffic.", whatOf(&d, p))
}

// Several members with the symptom are a group, never one of them.
func TestSymptomOfAGroupNamesEachMember(t *testing.T) {
	p := holderIncident(objID("finalizer", "shop", "example.com/cleanup"),
		detection.ModeStuckDeleting, objID("secret", "shop", "s"),
		objID("role", "shop", "r"), objID("rolebinding", "shop", "rb"),
		objID("serviceaccount", "shop", "sa"))
	d := toDetail(p)

	got := whatOf(&d, p)

	assert.Equal(t, "Each of 4 objects in shop is stuck deleting (Role "+
		"r, RoleBinding rb, Secret s and 1 more).", got)
	assert.NotContains(t, got, "Finalizer")
}

// The guard keeps what was right: a root that can have the symptom is
// its subject.
func TestSymptomStaysOnARootThatCanHaveIt(t *testing.T) {
	p := holderIncident(objID("service", "shop", "web"),
		detection.ModeNoEndpoints, objID("pod", "shop", "web-1"))
	d := toDetail(p)

	assert.Equal(t, "Service web in shop has no ready pods to send "+
		"traffic to.", whatOf(&d, p))
	assert.True(t, canHave(objID("Deployment", "shop", "a"), "CrashLoop"))
	assert.True(t, canHave(inventory.EntityID{Group: "example.com",
		Kind: "widget", Name: "w"}, "Scaling"))
	assert.False(t, canHave(objID("secret", "shop", "s"), "Error"))
	assert.False(t, canHave(objID("node", "", "n"), probeFamily))
	assert.False(t, canHave(objID("finalizer", "", "f"), "StuckDeleting"))
}

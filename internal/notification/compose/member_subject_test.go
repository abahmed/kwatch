package compose

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// rootedElsewhere is inc moved onto root, an object that is not the
// workload whose container has the finding: the root-cause side may
// group a container under its node or a Service.
func rootedElsewhere(inc incident.Incident,
	root inventory.EntityID) incident.Incident {
	inc.Root = root
	return inc
}

// A node is never titled with a container's CPU or readiness sentence:
// only the subject's own container's finding is "its" (owns).
func TestNodeLeadNeverTakesAContainersSentence(t *testing.T) {
	node := inventory.CoreID(kube.KindNode, "", "node-a")
	ready := probeIncident("ReadinessProbeFailed",
		detection.Evidence{Label: detection.EvidenceProbeCheck,
			Value: "http /readyz :8081"},
		detection.Evidence{Label: detection.EvidenceProbeResult,
			Value: "HTTP 500"})
	for name, p := range map[string]incident.Incident{
		"arch":  rootedElsewhere(archIncident(""), node),
		"probe": rootedElsewhere(ready, node),
	} {
		title := NewWriter("", nil).ProblemTitle(p, nil, writerNow)

		assert.NotContains(t, title, "its image", name)
		assert.NotContains(t, title, "its readiness check", name)
		assert.Contains(t, title, "node-a", name)
	}
}

// A Service root and a pod member: the pod's finding is said of the
// pod, never as the Service's own state.
func TestServiceLeadNamesThePodWhoseFindingItIs(t *testing.T) {
	svc := inventory.CoreID(kube.KindService, "shop", "lookup")
	pod := inventory.CoreID(kube.KindPod, "shop", "web-7d9f-a")
	p := incident.Incident{ID: "svc-1", Root: svc, Tier: incident.Notify,
		State: incident.Open, Opened: writerNow,
		Members: members(detection.Finding{Entity: pod,
			Reason: "Error", Severity: detection.Warning, Since: writerNow,
			Summary: "Pod exits with an error"})}

	title := NewWriter("", nil).ProblemTitle(p, nil, writerNow)

	assert.True(t, strings.HasPrefix(title, "Service lookup in shop is "+
		"failing; pod web-7d9f-a "), title)
	assert.NotContains(t, title, "lookup in shop exits")
}

// Without relations (an older record) the guard keeps only what a
// controller creates and names after itself: a name never makes an
// object of another kind the workload's.
func TestOwnsByNameKeepsCreatedObjectsOnly(t *testing.T) {
	deploy := inventory.CoreID(kube.KindDeployment, "shop", "web")
	cron := inventory.CoreID(kube.KindCronJob, "shop", "backup")
	f := caseFacts{}
	assert.True(t, f.owns(deploy, inventory.CoreID(kube.KindPod, "shop",
		"web-7d9f-a")))
	assert.True(t, f.owns(deploy, inventory.EntityID{
		Kind: kube.KindContainer, Namespace: "shop", Name: "web-7d9f-a/c"}))
	assert.True(t, f.owns(cron, inventory.CoreID(kube.KindJob, "shop",
		"backup-2989")))
	assert.False(t, f.owns(deploy, inventory.CoreID(kube.KindService,
		"shop", "web")))
	assert.False(t, f.owns(deploy, inventory.CoreID(kube.KindHPA, "shop",
		"web")))
	assert.False(t, f.owns(deploy, inventory.CoreID(kube.KindConfigMap,
		"shop", "web-settings")))
	assert.False(t, f.owns(deploy, inventory.CoreID(kube.KindPod, "other",
		"web-7d9f-a")))
	assert.False(t, f.owns(inventory.CoreID(kube.KindNode, "", "web"),
		inventory.CoreID(kube.KindPod, "shop", "web-1")))
	assert.False(t, f.owns(inventory.CoreID(kube.KindSecret, "shop", "web"),
		inventory.CoreID(kube.KindPod, "shop", "web-1")))
}

// With relations, ownership follows them: the workload's own
// autoscaler and the Service selecting its pods are its own, whatever
// their names; a same-named object of another workload is not.
func TestOwnsFollowsRelations(t *testing.T) {
	deploy := inventory.CoreID(kube.KindDeployment, "shop", "web")
	other := inventory.CoreID(kube.KindDeployment, "shop", "web-admin")
	hpa := inventory.CoreID(kube.KindHPA, "shop", "frontend")
	svc := inventory.CoreID(kube.KindService, "shop", "web")
	pod := inventory.CoreID(kube.KindPod, "shop", "web-admin-5c-x")
	f := caseFacts{p: incident.Incident{Belongs: incident.Belongs{
		hpa: deploy, svc: other, pod: other}}}
	assert.True(t, f.owns(deploy, hpa))
	assert.False(t, f.owns(deploy, svc))
	assert.False(t, f.owns(deploy, pod), "named like web, owned by web-admin")
	assert.True(t, f.owns(other, pod))
}

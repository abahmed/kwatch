package dashboard

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
)

// A root that runs no code never gets the self-cause sentence: it is
// the cause of the members it fails, or its own finding is said, or
// there is no "Why?".
func TestSelfCauseIsOnlyForKindsThatRunCode(t *testing.T) {
	secret := objID("secret", "shop", "db-creds")
	p := holderIncident(secret, detection.ModeMissingSecret,
		objID("pod", "shop", "db-creds-5c57-x"))
	d := toDetail(p)
	d.RootCause = "secret db-creds explains 1 failure"

	got := whyText(&d, p).Text
	assert.Equal(t, "The secret db-creds is the cause.", got)
	assert.NotContains(t, got, noOutsideCause)

	svc := objID("service", "shop", "hook")
	p = holderIncident(svc, detection.ModeNoEndpoints)
	p.State = incident.Resolved
	p.Members[detection.Key{Entity: svc, Reason: "x"}] =
		detection.Finding{Entity: svc, Summary: "Service has no ready " +
			"backends; traffic to it fails"}
	d = toDetail(p)
	d.RootCause = "service hook explains 1 failure"
	assert.Equal(t, "What kwatch found on the service hook: Service has "+
		"no ready backends; traffic to it fails.", whyText(&d, p).Text)

	p = holderIncident(objID("apiservice", "", "v1.metrics.example.io"),
		detection.ModeScalingNoMetrics)
	d = toDetail(p)
	d.RootCause = "apiservice v1.metrics.example.io explains 1 failure"
	assert.Equal(t, SummaryWhy{}, whyText(&d, p))
}

// An object named by a hash is said through what it joins whenever its
// finding names them, the same way for every such object.
func TestOpaqueNamedRootIsSaidByWhatItJoins(t *testing.T) {
	root := objID("volumeattachment", "", "csi-5468c63f")
	text := "Attachment of volume pv-1 to node node-a is stuck " +
		"deleting; its node and volume no longer exist"
	for _, mode := range []detection.Mode{detection.ModeStuckDeleting,
		"VolumeDetach"} {
		p := holderIncident(root, mode)
		p.Members[detection.Key{Entity: root, Reason: "detach"}] =
			detection.Finding{Entity: root, Mode: mode, Summary: text,
				Evidence: []detection.Evidence{
					{Label: detection.EvidenceAttachVolume, Value: "pv-1"},
					{Label: detection.EvidenceAttachNode,
						Value: "node-a"}}}
		p.Members[detection.Key{Entity: root, Reason: "held"}] =
			detection.Finding{Entity: root, Mode: mode,
				Summary: "Deletion is blocked by finalizers"}
		d := toDetail(p)

		assert.Equal(t, text+".", whatOf(&d, p), string(mode))
	}
}

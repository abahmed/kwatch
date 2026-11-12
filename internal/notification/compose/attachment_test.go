package compose

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func attachmentFacts(gone string) caseFacts {
	id := inventory.CoreID(kube.KindVolumeAttachment, "", "csi-9f8e7d")
	lead := detection.Finding{Entity: id,
		Reason: reasons.VolumeDetachFailure,
		Evidence: []detection.Evidence{
			{Label: detection.EvidenceAttachVolume, Value: "pvc-1234"},
			{Label: detection.EvidenceAttachNode, Value: "ip-10-0-1-5"},
			{Label: detection.EvidenceAttachDriver,
				Value: "ebs.csi.example.com"},
			{Label: detection.EvidenceAttachGone, Value: gone}}}
	return caseFacts{p: incident.Incident{Root: id},
		members: []detection.Finding{lead}, lead: lead, ok: true,
		cluster: "prod-eu-1"}
}

func TestAttachmentLeadNamesTheVolumeNotTheHash(t *testing.T) {
	cases := map[string]string{
		"volume": "Attachment of volume pvc-1234 to node ip-10-0-1-5 by " +
			"ebs.csi.example.com (prod-eu-1) is stuck deleting; the " +
			"volume pvc-1234 it attaches no longer exists.",
		"node": "Attachment of volume pvc-1234 to node ip-10-0-1-5 by " +
			"ebs.csi.example.com (prod-eu-1) is stuck deleting; its " +
			"node no longer exists.",
		"volume and node": "Attachment of volume pvc-1234 to node " +
			"ip-10-0-1-5 by ebs.csi.example.com (prod-eu-1) is stuck " +
			"deleting; its node and volume no longer exist, so " +
			"nothing is left to detach it.",
	}
	for gone, want := range cases {
		got := leadSentences(attachmentFacts(gone))
		if len(got) != 1 || got[0].text != want {
			t.Errorf("%s: got %+v\nwant %q", gone, got, want)
		}
	}
}

// An attachment without the evidence keeps the generic lead.
func TestAttachmentLeadNeedsItsEvidence(t *testing.T) {
	f := attachmentFacts("volume")
	f.lead.Evidence = nil

	if _, ok := attachmentLead(f); ok {
		t.Fatal("worded an attachment that names no volume")
	}
}

// An attachment whose node and volume are gone gets the checks, then
// the one change that ends it, marked as changing the cluster.
func TestOrphanedAttachmentStepsRemoveItsFinalizerLast(t *testing.T) {
	f := attachmentFacts("volume and node")

	steps := causeSteps(f.p, f.members)

	if len(steps) != 3 {
		t.Fatalf("steps = %+v", steps)
	}
	if steps[0].Command != "kubectl get node ip-10-0-1-5" ||
		steps[1].Command != "kubectl get pv pvc-1234" ||
		steps[0].Mutating || steps[1].Mutating {
		t.Errorf("checks = %+v", steps[:2])
	}
	want := "kubectl patch volumeattachment csi-9f8e7d --type=merge " +
		`-p '{"metadata":{"finalizers":null}}'`
	if steps[2].Command != want || !steps[2].Mutating {
		t.Errorf("fix = %+v", steps[2])
	}

	f = attachmentFacts("volume")
	if got := causeSteps(f.p, f.members); len(got) > 0 &&
		got[len(got)-1].Mutating {
		t.Errorf("only the volume is gone: no patch, got %+v", got)
	}
}

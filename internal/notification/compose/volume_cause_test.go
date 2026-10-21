package compose

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func volumeHeldFacts(evidence ...detection.Evidence) caseFacts {
	pod := inventory.CoreID(kube.KindPod, "shop", "api-7f")
	old := inventory.CoreID(kube.KindPod, "shop", "api-6d")
	api := inventory.CoreID(kube.KindDeployment, "shop", "api")
	wait := detection.Finding{Entity: pod,
		Reason: reasons.VolumeMultiAttach, Evidence: evidence}
	return caseFacts{
		p: incident.Incident{Root: api, Cause: &rootcause.CauseRecord{
			Rule: "volume-held-by-pod", Root: old,
			Chain: []inventory.EntityID{old, pod, api}}},
		members: []detection.Finding{wait}, lead: wait, ok: true}
}

func TestVolumeHeldLeadNamesWhatHoldsTheVolume(t *testing.T) {
	f := volumeHeldFacts(
		detection.Evidence{Label: detection.EvidenceHeldVolume,
			Value: "pvc-1"},
		detection.Evidence{Label: detection.EvidenceAttachNode, Value: "n1"},
		detection.Evidence{Label: detection.EvidenceHeldPod,
			Value: "api-6d"},
		detection.Evidence{Label: detection.EvidenceHeldPodEnding,
			Value: "terminating"})

	got, ok := volumeHeldLead(f)

	want := "can't start: its volume pvc-1 is still attached to node n1 " +
		"(used by pod api-6d, terminating)"
	if !ok || !strings.HasSuffix(got, want) {
		t.Fatalf("got %q (%v), want suffix %q", got, ok, want)
	}
}

func TestVolumeHeldLeadLeavesOutWhatIsUnknown(t *testing.T) {
	got, ok := volumeHeldLead(volumeHeldFacts())

	if !ok || !strings.HasSuffix(got, "its volume is still attached to "+
		"another node") {
		t.Fatalf("got %q (%v)", got, ok)
	}
}

func provisionerFacts(rule, runs string) caseFacts {
	claim := inventory.CoreID(kube.KindPVC, "data", "orders-db")
	pending := detection.Finding{Entity: claim,
		Reason: reasons.PersistentVolumeClaim,
		Evidence: []detection.Evidence{
			{Label: detection.EvidenceProvisioner, Value: "disk.csi.example.com"},
			{Label: detection.EvidenceProvisionerWorkload, Value: runs}}}
	failed := detection.Finding{Entity: claim,
		Reason: reasons.ProvisioningFailed,
		Evidence: []detection.Evidence{{Label: "event",
			Value: "rpc error: could not create the disk"}}}
	return caseFacts{
		p: incident.Incident{Root: claim, Cause: &rootcause.CauseRecord{
			Rule: rule, Root: claim}},
		members: []detection.Finding{pending, failed}, lead: pending,
		ok: true}
}

func TestProvisionerSentenceQuotesTheErrorWhenNothingServesIt(t *testing.T) {
	got := provisionerSentences(provisionerFacts("self", "none found"))

	if len(got) != 1 || !strings.Contains(plainQuotes(got[0].text),
		"kwatch sees no "+
			"workload in the cluster serving its provisioner "+
			"disk.csi.example.com (it may run outside the cluster); the last "+
			"provisioning failure says "+
			`"rpc error: could not create the disk"`) {
		t.Fatalf("got %+v", got)
	}
}

func TestProvisionerLeadNamesTheWorkloadWithNoReadyPods(t *testing.T) {
	f := provisionerFacts("provisioner-down",
		"deployment kube-system/ebs-csi-controller, no ready pods")

	got, ok := provisionerLead(f)

	if !ok || !strings.HasSuffix(got, "is Pending: its provisioner "+
		"disk.csi.example.com runs as deployment kube-system/"+
		"ebs-csi-controller, which has no ready pods") {
		t.Fatalf("got %q (%v)", got, ok)
	}
	if said := provisionerSentences(f); len(said) != 0 {
		t.Fatalf("the lead already says it, got %+v", said)
	}
}

func TestProvisionerSentenceIgnoresAWorkloadWithReadyPods(t *testing.T) {
	f := provisionerFacts("self", "deployment kube-system/ctl, 2 ready")

	if got := provisionerSentences(f); len(got) != 0 {
		t.Fatalf("blamed a provisioner that runs: %+v", got)
	}
}

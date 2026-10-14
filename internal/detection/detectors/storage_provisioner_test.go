package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// pendingClaimOfClass is a claim Pending for five minutes whose class
// names provisioner. It returns the model and the claim.
func pendingClaimOfClass(
	provisioner string,
) (*inventory.Model, inventory.Entity) {
	m := newTestModel()
	class := newID(kube.KindStorageClass, "", "gp")
	claimID := newID(kube.KindPVC, "data", "orders-db")
	put(m, class, t0, map[string]inventory.Value{
		kube.AttrProvisioner: inventory.Text(provisioner)})
	put(m, claimID, t0, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Pending")})
	link(m, claimID, inventory.References, class)
	claim, _ := m.Entity(claimID)
	return m, claim
}

func claimEvidence(
	m *inventory.Model, claim inventory.Entity,
) []detection.Evidence {
	ctx := testDetectorContext(m, t0.Add(5*time.Minute))
	found := Claim{}.Detect(ctx, claim)
	if len(found) == 0 {
		return nil
	}
	return found[0].Evidence
}

func TestClaimPendingNamesItsProvisionerAndWhatRunsIt(t *testing.T) {
	m, claim := pendingClaimOfClass("ebs.csi.example.com")
	ctl := newID(kube.KindDeployment, "kube-system", "ebs-csi-controller")
	rs := newID(kube.KindReplicaSet, "kube-system", "ebs-csi-controller-1")
	pod := newID(kube.KindPod, "kube-system", "ebs-csi-controller-1-a")
	put(m, ctl, t0, map[string]inventory.Value{
		kube.AttrReadyReplicas: inventory.Number(0)})
	put(m, rs, t0, nil)
	put(m, pod, t0, nil)
	link(m, rs, inventory.OwnedBy, ctl)
	link(m, pod, inventory.OwnedBy, rs)

	got := claimEvidence(m, claim)

	require.NotEmpty(t, got)
	assert.Contains(t, got, detection.Evidence{
		Label: detection.EvidenceProvisioner, Value: "ebs.csi.example.com"})
	assert.Contains(t, got, detection.Evidence{
		Label: detection.EvidenceProvisionerWorkload,
		Value: "deployment kube-system/ebs-csi-controller, no ready pods"})
}

func TestClaimPendingSaysWhenNoWorkloadServesTheProvisioner(t *testing.T) {
	m, claim := pendingClaimOfClass("disk.csi.example.com")

	got := claimEvidence(m, claim)

	assert.Contains(t, got, detection.Evidence{
		Label: detection.EvidenceProvisionerWorkload, Value: "none found"})
}

func TestClaimPendingSaysNothingOfAnInTreeProvisioner(t *testing.T) {
	m, claim := pendingClaimOfClass("kubernetes.io/aws-ebs")

	for _, e := range claimEvidence(m, claim) {
		assert.NotEqual(t, detection.EvidenceProvisionerWorkload, e.Label)
	}
}

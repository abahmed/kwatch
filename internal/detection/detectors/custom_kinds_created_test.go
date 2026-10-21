package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func createdCSR(m *inventory.Model, created time.Time) inventory.EntityID {
	return customEntity(m, "certificatesigningrequest", "", "csr-1",
		map[string]inventory.Value{
			kube.AttrSignerName:        inventory.Text(kubeletServingSigner),
			kube.AttrCertificateIssued: inventory.Bool(false),
			kube.AttrCreated:           inventory.Time(created),
		})
}

// A kubelet serving request waits for approval from its creation, not
// from the moment kwatch saw it: after a restart of kwatch it is not
// new again, and the finding does not resolve and reopen with it.
func TestCustomServingRequestWaitsFromItsCreation(t *testing.T) {
	m := newTestModel()
	created := t0.Add(-DefaultServingApproval - 5*time.Minute)
	id := createdCSR(m, created)

	got := evaluate(Custom{}, m, t0, id, nil).Findings

	require.Len(t, got, 1)
	assert.Equal(t, reasons.CertificateNotIssued, got[0].Reason)
	assert.True(t, created.Equal(got[0].Since), got[0].Since.String())
}

func TestCustomServingRequestYoungerThanTheGraceWaits(t *testing.T) {
	m := newTestModel()
	id := createdCSR(m, t0.Add(-5*time.Minute))

	got := evaluate(Custom{}, m, t0, id, nil)

	assert.Empty(t, got.Findings)
	assert.Equal(t, DefaultServingApproval-5*time.Minute,
		got.RecheckAfter)
}

func createdClaim(
	m *inventory.Model, created time.Time,
) inventory.EntityID {
	pod := newID(kube.KindPod, "ml", "train-0")
	put(m, pod, t0, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Pending"),
	})
	claim := customEntity(m, "resourceclaim", "ml", "train-0-gpu",
		map[string]inventory.Value{
			kube.AttrAllocated: inventory.Bool(false),
			kube.AttrCreated:   inventory.Time(created),
		})
	link(m, claim, inventory.OwnedBy, pod)
	return claim
}

// A claim without a device counts its wait from its creation, so a
// restart of kwatch does not make a long-waiting pod new again.
func TestCustomUnallocatedClaimWaitsFromItsCreation(t *testing.T) {
	m := newTestModel()
	created := t0.Add(-DefaultClaimUnallocated - 5*time.Minute)
	claim := createdClaim(m, created)

	got := evaluate(Custom{}, m, t0, claim, nil).Findings

	require.Len(t, got, 1)
	assert.Equal(t, reasons.ResourceClaimUnallocated, got[0].Reason)
	assert.True(t, created.Equal(got[0].Since), got[0].Since.String())
}

func TestCustomUnallocatedClaimYoungerThanTheGraceWaits(t *testing.T) {
	m := newTestModel()
	claim := createdClaim(m, t0.Add(-time.Minute))

	got := evaluate(Custom{}, m, t0, claim, nil)

	assert.Empty(t, got.Findings)
	assert.Equal(t, DefaultClaimUnallocated-time.Minute, got.RecheckAfter)
}

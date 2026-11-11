package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// policyBackend is the gone Service of a removed policy operator.
var policyBackend = inventory.CoreID(kube.KindService, "policy-system",
	"policy-svc")

// leftoverSig is a finding left behind by policyBackend.
func leftoverSig(
	kind inventory.Kind, name, why string, sev detection.Severity,
) detection.Finding {
	s := sig(inventory.CoreID(kind, "policy-system", name), why, sev)
	s.Leftover = policyBackend
	return s
}

// The leftovers of one removed controller are one incident rooted at
// its absent backend, at the digest tier.
func TestLeftoversOfOneControllerAreOneIncident(t *testing.T) {
	r := newRig(t, Config{})
	hook := leftoverSig(kube.KindValidatingHook, "policy-hook",
		reasons.WebhookBackendNotFound, detection.Info)
	hook.Entity.Namespace = ""
	certs := []detection.Finding{
		leftoverSig(kube.KindSecret, "policy-tls", reasons.TLSCertExpired,
			detection.Warning),
		leftoverSig(kube.KindSecret, "cleanup-tls",
			reasons.TLSCertExpired, detection.Warning),
	}
	r.raise(at(0), append(certs, hook)...)

	p := r.only()
	assert.Equal(t, policyBackend, p.Root)
	assert.Len(t, p.Members, 3)
	assert.Equal(t, Digest, p.Tier)
	members, ok := p.Leftovers()
	require.True(t, ok)
	assert.Len(t, members, 3)
	assert.Equal(t, 3, p.LeftoverObjects())
}

// A leftover that would be louder on its own keeps its own incident.
func TestLeftoverThatBlocksKeepsItsOwnIncident(t *testing.T) {
	r := newRig(t, Config{})
	quiet := leftoverSig(kube.KindSecret, "policy-tls",
		reasons.TLSCertExpired, detection.Warning)
	other := leftoverSig(kube.KindSecret, "cleanup-tls",
		reasons.TLSCertExpired, detection.Warning)
	loud := leftoverSig(kube.KindDeployment, "policy-api",
		reasons.CrashLoopBackOff, detection.Warning)
	r.raise(at(0), quiet, other, loud)

	assert.Equal(t, Digest, r.of(policyBackend).Tier)
	own := r.of(loud.Entity)
	assert.Equal(t, Notify, own.Tier)
	_, ok := own.Leftovers()
	assert.False(t, ok, "not a leftover group")
}

// An incident whose members were not left behind by its root is no
// leftover group.
func TestOrdinaryIncidentHasNoLeftovers(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), podSig("web"))

	_, ok := r.only().Leftovers()
	assert.False(t, ok)
	assert.Zero(t, r.only().LeftoverObjects())
}

// A refreshed member keeps the mark the incident layer gave it.
func TestRefreshKeepsTheLeftoverMark(t *testing.T) {
	r := newRig(t, Config{})
	cert := leftoverSig(kube.KindSecret, "policy-tls",
		reasons.TLSCertExpired, detection.Warning)
	r.raise(at(0), cert, leftoverSig(kube.KindSecret, "cleanup-tls",
		reasons.TLSCertExpired, detection.Warning))
	plain := cert
	plain.Leftover = inventory.EntityID{}
	r.apply(at(time.Minute), detection.Refreshed, plain)

	assert.Equal(t, policyBackend,
		r.of(policyBackend).Members[cert.Key()].Leftover)
}

// Objects stuck on the finalizer of a stopped controller live in
// another namespace than the controller. When the engine blames that
// controller and its namespace has an absent backend, they join the
// controller's leftovers.
func TestStuckObjectsOfAStoppedControllerJoinItsLeftovers(t *testing.T) {
	r := newRig(t, Config{})
	controller := inventory.CoreID(kube.KindDeployment, "policy-system",
		"policy-controller")
	stuck := sig(inventory.CoreID(kube.KindSecret, "runners", "token"),
		reasons.StuckDeleting, detection.Info)
	stuck.Mode = detection.ModeStuckDeleting
	r.cause(stuck.Entity, controller, "scaled to zero")
	var certs []detection.Finding
	for _, name := range []string{"policy-tls", "cleanup-tls"} {
		cert := leftoverSig(kube.KindSecret, name,
			reasons.TLSCertExpired, detection.Warning)
		cert.Leftover = controller
		certs = append(certs, cert)
	}
	r.raise(at(0), append(certs, stuck)...)

	p := r.of(controller)
	assert.Len(t, p.Members, 3)
	assert.Equal(t, 3, p.LeftoverObjects())
	assert.NotNil(t, p.Cause, "the engine's cause is kept")
}

// A leftover alone keeps its own incident; once its backend leaves a
// second object behind, both move to the backend's group.
func TestLoneLeftoverJoinsTheGroupWhenASecondArrives(t *testing.T) {
	r := newRig(t, Config{})
	first := leftoverSig(kube.KindSecret, "policy-tls",
		reasons.TLSCertExpired, detection.Warning)
	r.raise(at(0), first)
	assert.Equal(t, first.Entity, r.only().Root)

	r.raise(at(time.Minute), leftoverSig(kube.KindSecret, "cleanup-tls",
		reasons.TLSCertExpired, detection.Warning))

	p := r.of(policyBackend)
	assert.Len(t, p.Members, 2)
	assert.Equal(t, 2, p.LeftoverObjects())
}

// Without an absent backend in the controller's namespace the stuck
// objects stay where the engine placed them.
func TestStuckObjectsWithoutLeftoversStayWithTheirCause(t *testing.T) {
	r := newRig(t, Config{})
	controller := inventory.CoreID(kube.KindDeployment, "policy-system",
		"policy-controller")
	stuck := sig(inventory.CoreID(kube.KindSecret, "runners", "token"),
		reasons.StuckDeleting, detection.Info)
	stuck.Mode = detection.ModeStuckDeleting
	r.cause(stuck.Entity, controller, "scaled to zero")
	r.raise(at(0), stuck)

	assert.Equal(t, controller, r.only().Root)
}

// A lone old leftover is left out as an old problem. When its backend
// leaves a second object behind, the group forms and takes it in: the
// group names every object.
func TestAgedOutLeftoverJoinsTheGroupWhenItForms(t *testing.T) {
	r := newRig(t, Config{OldProblem: 30 * 24 * time.Hour})
	first := leftoverSig(kube.KindSecret, "policy-tls",
		reasons.TLSCertExpired, detection.Warning)
	first.Since = t0.Add(-400 * 24 * time.Hour)
	r.raise(at(0), first)
	require.Empty(t, r.m.Export())
	require.Len(t, r.m.OldProblems(), 1)

	second := leftoverSig(kube.KindSecret, "cleanup-tls",
		reasons.TLSCertExpired, detection.Warning)
	second.Since = first.Since
	r.raise(at(time.Minute), second)

	assert.Len(t, r.of(policyBackend).Members, 2)
	assert.Empty(t, r.m.OldProblems())
}

// When the other leftovers clear, the last one stays in the live
// group: it does not move to an incident of its own.
func TestLastLeftoverStaysInTheLiveGroup(t *testing.T) {
	r := newRig(t, Config{})
	first := leftoverSig(kube.KindSecret, "policy-tls",
		reasons.TLSCertExpired, detection.Warning)
	second := leftoverSig(kube.KindSecret, "cleanup-tls",
		reasons.TLSCertExpired, detection.Warning)
	r.raise(at(0), first, second)
	group := r.only().ID
	r.clear(at(time.Minute), second)

	first.Summary = "expired 401 days ago"
	r.apply(at(2*time.Minute), detection.Changed, first)

	p := r.only()
	assert.Equal(t, group, p.ID)
	assert.Equal(t, policyBackend, p.Root)
	assert.Contains(t, p.Members, first.Key())
}

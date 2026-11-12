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
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// runnerController is a stopped controller whose objects in namespace
// runners wait on its finalizer.
var runnerController = inventory.CoreID(kube.KindDeployment, "runners",
	"runner-controller")

// stuckOn is a low stuck deletion of a Secret in runners, held by
// finalizer.
func stuckOn(name, finalizer string) detection.Finding {
	s := sig(inventory.CoreID(kube.KindSecret, "runners", name),
		reasons.StuckDeleting, detection.Info)
	s.Mode = detection.ModeStuckDeleting
	s.Evidence = []detection.Evidence{
		{Label: detection.EvidenceFinalizers, Value: finalizer}}
	return s
}

// runnerGroup raises the controller's leftover group: two expired
// certificates and one object stuck on its finalizer.
func runnerGroup(r *rig) {
	var group []detection.Finding
	for _, name := range []string{"runner-tls", "runner-ca"} {
		cert := sig(inventory.CoreID(kube.KindSecret, "runners", name),
			reasons.TLSCertExpired, detection.Warning)
		cert.Leftover = runnerController
		group = append(group, cert)
	}
	first := stuckOn("token-a", "example.com/cleanup")
	first.Leftover = runnerController
	r.raise(at(0), append(group, first)...)
	require.Len(r.t, r.m.Export(), 1)
}

// An object stuck on the finalizer the leftover group's objects wait
// on joins the group at once: no incident of its own opens and
// resolves seconds later, neither when it is raised nor when the
// engine then blames the finalizer.
func TestStuckObjectJoinsTheGroupOfItsFinalizerAtOnce(t *testing.T) {
	r := newRig(t, Config{})
	runnerGroup(r)
	second := stuckOn("token-b", "example.com/cleanup")

	r.raise(at(time.Minute), second)

	require.Len(t, r.m.Export(), 1, "no incident of its own")
	p := r.of(runnerController)
	require.Contains(t, p.Members, second.Key())
	assert.Equal(t, 4, p.LeftoverObjects())

	finalizer := inventory.CoreID(explain.KindFinalizer, "runners",
		"example.com/cleanup")
	r.cause(second.Entity, finalizer, "nothing removes the finalizer")
	second.Summary = "still blocked"
	r.apply(at(2*time.Minute), detection.Changed, second)

	require.Len(t, r.m.Export(), 1)
	assert.Contains(t, r.of(runnerController).Members, second.Key())
}

// Without a leftover group, a stuck object joins the incident that
// holds the others stuck on its finalizer.
func TestStuckObjectJoinsItsFinalizerIncident(t *testing.T) {
	r := newRig(t, Config{})
	finalizer := inventory.CoreID(explain.KindFinalizer, "runners",
		"example.com/cleanup")
	first := stuckOn("token-a", "example.com/cleanup")
	r.cause(first.Entity, finalizer, "nothing removes the finalizer")
	r.raise(at(0), first)
	require.Equal(t, finalizer, r.only().Root)

	second := stuckOn("token-b", "example.com/cleanup")
	r.raise(at(time.Minute), second)

	require.Len(t, r.m.Export(), 1)
	assert.Contains(t, r.of(finalizer).Members, second.Key())
}

// An object held by another finalizer stays apart.
func TestStuckObjectOfAnotherFinalizerStaysApart(t *testing.T) {
	r := newRig(t, Config{})
	runnerGroup(r)
	other := stuckOn("token-c", "example.com/other")

	r.raise(at(time.Minute), other)

	assert.Len(t, r.m.Export(), 2)
	assert.Equal(t, other.Entity, r.of(other.Entity).Root)
}

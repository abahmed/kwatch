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

// observeAttrs puts id in r's model with attrs.
func (r *rig) observeAttrs(id inventory.EntityID, attrs map[string]string) {
	r.t.Helper()
	values := map[string]inventory.Value{}
	for name, text := range attrs {
		values[name] = inventory.Text(text)
	}
	_, err := r.model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: "test", At: t0, Entity: id,
		Attributes: values,
	})
	require.NoError(r.t, err)
}

// meshID is an object of the removed mesh in mesh-system.
func meshID(kind inventory.Kind, name string) inventory.EntityID {
	return inventory.CoreID(kind, "mesh-system", name)
}

// meshLeftovers are what a removed mesh control plane left: an
// autoscaler of its gone Deployment, a budget selecting its pods and an
// injector webhook whose Service has no pods.
func meshLeftovers(r *rig) []detection.Finding {
	hpa := meshID(kube.KindHPA, "mesh-control")
	r.observeAttrs(hpa, nil)
	r.relate(hpa, inventory.Scales,
		meshID(kube.KindDeployment, "mesh-control"))
	pdb := meshID(kube.KindPDB, "mesh-control")
	r.observeAttrs(pdb, map[string]string{kube.AttrSelector: "app=mesh-control"})
	hook := inventory.CoreID(kube.KindMutatingWebhook, "", "mesh-injector")
	r.observeAttrs(hook, map[string]string{
		kube.AttrWebhookNames: "sidecar.mesh.example.com"})
	webhook := sig(hook, reasons.WebhookNoEndpoints, detection.Info)
	webhook.Leftover = meshID(kube.KindService, "mesh-control")
	return []detection.Finding{
		sig(hpa, reasons.HPATargetMissing, detection.Info),
		sig(pdb, reasons.PdbSelectsNothing, detection.Warning),
		webhook,
	}
}

// The leftovers of one removed controller, found by three detectors,
// are one Low incident rooted at the gone Deployment.
func TestLeftoversOfOneGoneControllerAreOneIncident(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), meshLeftovers(r)...)

	p := r.only()
	assert.Equal(t, meshID(kube.KindDeployment, "mesh-control"), p.Root)
	assert.Equal(t, Digest, p.Tier)
	assert.Equal(t, 3, p.LeftoverObjects())
}

// A live failure in the same namespace keeps its own incident.
func TestLiveFailureIsNeverALeftover(t *testing.T) {
	r := newRig(t, Config{})
	live := sig(meshID(kube.KindDeployment, "mesh-gateway"),
		reasons.CrashLoopBackOff, detection.Warning)
	r.raise(at(0), append(meshLeftovers(r), live)...)

	assert.Len(t, r.m.Export(), 2)
	p := r.of(live.Entity)
	assert.True(t, p.Members[live.Key()].Leftover.IsZero())
}

// Workloads parked at zero replicas that routes still reach are one
// Low incident for their namespace; one alone keeps its own.
func TestParkedWorkloadsOfANamespaceAreOneIncident(t *testing.T) {
	parked := func(name string) detection.Finding {
		return sig(inventory.CoreID(kube.KindDeployment, "demo", name),
			reasons.ScaledToZeroRouted, detection.Info)
	}
	r := newRig(t, Config{})
	r.raise(at(0), parked("cart"))
	assert.Equal(t, parked("cart").Entity, r.only().Root)

	r.raise(at(time.Minute), parked("search"))

	p := r.of(inventory.CoreID(kube.KindNamespace, "", "demo"))
	assert.Len(t, p.Members, 2)
	assert.Empty(t, r.of(parked("cart").Entity).Members,
		"the first one moved to the group")
}

// A Helm release stuck uninstalling and an expired certificate join
// the webhooks of the Service they are named after: the Service roots
// the group.
func TestReleaseAndCertificateJoinTheirService(t *testing.T) {
	r := newRig(t, Config{})
	service := inventory.CoreID(kube.KindService, "guard", "guard-svc")
	var findings []detection.Finding
	for _, name := range []string{"guard-a", "guard-b"} {
		f := sig(inventory.CoreID(kube.KindValidatingHook, "", name),
			reasons.WebhookBackendNotFound, detection.Info)
		f.Leftover = service
		findings = append(findings, f)
	}
	release := kube.HelmReleaseID("guard", "guard")
	cert := inventory.CoreID(kube.KindSecret, "guard", "guard-svc.tls")
	findings = append(findings,
		sig(release, reasons.HelmReleaseStuck, detection.Info),
		sig(cert, reasons.TLSCertExpired, detection.Warning))

	r.raise(at(0), findings...)

	p := r.only()
	assert.Equal(t, service, p.Root)
	assert.Equal(t, 4, p.LeftoverObjects())
}

// Objects stuck on a finalizer of the API domain a leftover webhook
// serves are that controller's leftovers too, in any namespace.
func TestStuckObjectsJoinTheWebhookOfTheirDomain(t *testing.T) {
	r := newRig(t, Config{})
	service := inventory.CoreID(kube.KindService, "runner-system", "hook")
	hook := inventory.CoreID(kube.KindMutatingWebhook, "", "runner")
	r.observeAttrs(hook, map[string]string{
		kube.AttrWebhookNames: "mutate.runs.example.com"})
	webhook := sig(hook, reasons.WebhookNoEndpoints, detection.Info)
	webhook.Leftover = service
	stuck := stuckOn("token-a", "runs.example.com/cleanup")

	r.raise(at(0), webhook, stuck)

	p := r.only()
	assert.Equal(t, service, p.Root)
	assert.Contains(t, p.Members, stuck.Key())
}

// Two budgets that select nothing, and nothing else, name no gone
// controller: they stay apart.
func TestBudgetsAloneDoNotLink(t *testing.T) {
	r := newRig(t, Config{})
	var findings []detection.Finding
	for _, name := range []string{"a", "b"} {
		pdb := inventory.CoreID(kube.KindPDB, "shop", name)
		r.observeAttrs(pdb, map[string]string{kube.AttrSelector: "app=web"})
		findings = append(findings,
			sig(pdb, reasons.PdbSelectsNothing, detection.Warning))
	}
	r.raise(at(0), findings...)

	assert.Len(t, r.m.Export(), 2)
}

func TestSelectorValuesReadEqualityTerms(t *testing.T) {
	assert.Equal(t, []string{"mesh", "control"},
		selectorValues("app=mesh,tier=control,zone!=a,env in (x)"))
	assert.Equal(t, []string{"web"}, selectorValues("app==web"))
}

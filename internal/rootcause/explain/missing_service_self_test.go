package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// certControllerFixture is the external-secrets cert-controller of the
// field: one pod whose own readiness check fails while it waits for its
// webhook certificates. Its configuration names no Service it calls;
// calls is what its calls attribute holds (an older model kept words
// that are its own name, and one that is no Service name at all).
func certControllerFixture(
	f *fixture, calls string,
) inventory.EntityID {
	const ns = "external-secrets"
	f.add(inventory.CoreID(kube.KindNamespace, "", ns),
		inventory.CoreID(kube.KindService, ns, "external-secrets-webhook"))
	pod := f.workload(ns, "external-secrets-cert-controller", 1)[0]
	attrs := map[string]inventory.Value{
		kube.AttrLabels: inventory.Text("app.kubernetes.io/instance=" +
			"external-secrets,app.kubernetes.io/name=" +
			"external-secrets-cert-controller"),
	}
	if calls != "" {
		attrs[kube.AttrServiceCalls] = inventory.Text(calls)
	}
	f.apply(inventory.Observation{Kind: inventory.Observed, Entity: pod,
		Attributes: attrs})
	f.fail(pod, detection.ModeNotReady, detection.Failing, 1,
		"containers with unready status: [app]")
	f.fail(containerOf(pod), detection.ModeProbeReadiness,
		detection.Failing, 1, "Readiness probe failed: HTTP probe "+
			"failed with statuscode: 500")
	return pod
}

func TestMissingServiceNamedAfterThePodIsNotBlamed(t *testing.T) {
	for name, calls := range map[string]string{
		"no calls": "",
		"own name": "external-secrets/external-secrets-cert-controller",
		"invalid name": "external-secrets/" +
			"owner=external-secrets-cert-controller",
		"both": "external-secrets/external-secrets-cert-controller," +
			"external-secrets/owner=external-secrets-cert-controller",
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			pod := certControllerFixture(f, calls)

			e := f.explain()

			for _, area := range e.Areas {
				for _, c := range append(area.Causes,
					area.Alternatives...) {
					if c.Root.Kind == kube.KindService {
						t.Fatalf("Service blamed: %s (%s)", c.Root,
							c.Row)
					}
				}
			}
			c := requireCause(t, e, containerOf(pod), "deployment/"+
				"external-secrets/external-secrets-cert-controller")
			if c.Row != "self" {
				t.Fatalf("row = %q, want the pod's own failure", c.Row)
			}
		})
	}
}

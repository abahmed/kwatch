package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// certControllerFixture is the secrets-system cert-controller of the
// field: one pod whose own readiness check fails while it waits for its
// webhook certificates. Its configuration names no Service it calls;
// calls is what its calls attribute holds (an older model kept words
// that are its own name, and one that is no Service name at all).
func certControllerFixture(
	f *fixture, calls string,
) inventory.EntityID {
	const ns = "secrets-system"
	f.add(inventory.CoreID(kube.KindNamespace, "", ns),
		inventory.CoreID(kube.KindService, ns, "secrets-webhook"))
	pod := f.workload(ns, "cert-controller", 1)[0]
	attrs := map[string]inventory.Value{
		kube.AttrLabels: inventory.Text("app.kubernetes.io/instance=" +
			"secrets-system,app.kubernetes.io/name=" +
			"cert-controller"),
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
		"own name": "secrets-system/cert-controller",
		"invalid name": "secrets-system/" +
			"owner=cert-controller",
		"both": "secrets-system/cert-controller," +
			"secrets-system/owner=cert-controller",
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
				"secrets-system/cert-controller")
			if c.Row != "self" {
				t.Fatalf("row = %q, want the pod's own failure", c.Row)
			}
		})
	}
}

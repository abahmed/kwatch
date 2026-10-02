package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A new namespace creates kube-root-ca.crt and the default ServiceAccount,
// and every pod references both through its token volume. Their creation
// must not make them the cause of the pod's own failure.
func TestExplainIgnoresBootstrapObjectCreation(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind inventory.Kind
		obj  string
	}{
		{"root CA ConfigMap", kube.KindConfigMap, "kube-root-ca.crt"},
		{"default ServiceAccount", kube.KindAccount, "default"},
		{"Secret", kube.KindSecret, "token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			pod := inventory.CoreID(kube.KindPod, "ns", "app")
			bootstrap := inventory.CoreID(tc.kind, "ns", tc.obj)
			f.add(pod, containerOf(pod), bootstrap)
			f.relate(containerOf(pod), inventory.PartOf, pod)
			f.relate(pod, inventory.References, bootstrap)
			f.change(bootstrap, 1)
			f.fail(pod, "CrashLoop", failingH, 2, "back-off restarting")

			requireCause(t, f.explain(), pod, pod.String())
		})
	}
}

func TestExplainKeepsDeletedConfigMapAsCause(t *testing.T) {
	f := newFixture(t)
	pod := inventory.CoreID(kube.KindPod, "ns", "app")
	cm := inventory.CoreID(kube.KindConfigMap, "ns", "settings")
	f.add(pod, containerOf(pod), cm)
	f.relate(containerOf(pod), inventory.PartOf, pod)
	f.relate(pod, inventory.References, cm)
	f.change(cm, 1, "data.mode")
	f.fail(pod, "CrashLoop", failingH, 2, "back-off restarting")

	requireCause(t, f.explain(), pod, cm.String())
}

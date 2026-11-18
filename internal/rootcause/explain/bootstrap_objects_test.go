package explain

import (
	"testing"
	"time"

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

// missingBootstrapCase has two pods that reference the default
// ServiceAccount of a namespace created at created, before kwatch saw
// the ServiceAccount.
func missingBootstrapCase(
	f *fixture, created time.Time,
) []inventory.EntityID {
	pods := f.workload("fresh", "api", 2)
	account := inventory.CoreID(kube.KindAccount, "fresh", "default")
	ns := inventory.CoreID(kube.KindNamespace, "", "fresh")
	f.apply(inventory.Observation{Kind: inventory.Observed, Entity: ns,
		Attributes: map[string]inventory.Value{
			kube.AttrPhase:   inventory.Text("Active"),
			kube.AttrCreated: inventory.Time(created)}})
	for _, pod := range pods {
		f.relate(pod, inventory.References, account)
		f.fail(pod, "Missing.ServiceAccount", failingH, 2, "")
	}
	return pods
}

// Kubernetes publishes the default ServiceAccount and kube-root-ca.crt
// a moment after a namespace: missing in its first moments they are
// late, not a cause. Missing well after, they are.
func TestExplainBootstrapObjectsAreLateInANewNamespace(t *testing.T) {
	for _, tc := range []struct {
		name   string
		age    time.Duration
		blamed bool
	}{
		{"new namespace", 30 * time.Second, false},
		{"old namespace", kube.BootstrapPublish + time.Minute, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			pods := missingBootstrapCase(f, f.now.Add(-tc.age))

			c, ok := f.explain().CauseOf(pods[0])

			got := ok && c.Root.String() == "serviceaccount/fresh/default"
			if got != tc.blamed {
				t.Fatalf("default account blamed = %v, want %v (cause %v)",
					got, tc.blamed, c.Root)
			}
		})
	}
}

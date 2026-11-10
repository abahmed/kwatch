package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// absentPullSecretCase crash-loops two pods of api that name a pull
// Secret nobody created. optional says whether the inventory marked
// the reference as one the pod starts without, as it does for a Secret
// named only in imagePullSecrets.
func absentPullSecretCase(
	f *fixture, optional bool,
) (secret inventory.EntityID) {
	pods := f.workload("shop", "api", 2)
	secret = inventory.CoreID(kube.KindSecret, "shop", "regcred")
	for _, pod := range pods {
		f.relate(pod, inventory.References, secret)
		attrs := map[string]inventory.Value{
			kube.AttrPullSecrets: inventory.Text("regcred")}
		if optional {
			attrs[kube.AttrOptionalRefs] = inventory.Text("secret/regcred")
		}
		f.observe(pod, attrs)
		f.fail(containerOf(pod), "CrashLoop", failingH, 3,
			"panic: template: unexpected EOF")
	}
	return secret
}

// The kubelet pulls without a pull Secret it cannot read. Pods that
// have their image and crash are not failing for want of it.
func TestAbsentPullSecretIsNotBlamedForACrash(t *testing.T) {
	f := newFixture(t)
	secret := absentPullSecretCase(f, true)
	if e := f.explain(); blames(e, secret) {
		t.Fatalf("absent pull secret blamed for a crash: %v", causes(e))
	}
}

// A Secret the pod also reads elsewhere (not marked optional) is still
// blamed when it is missing.
func TestAbsentSecretReadElsewhereIsStillBlamed(t *testing.T) {
	f := newFixture(t)
	secret := absentPullSecretCase(f, false)
	if e := f.explain(); !blames(e, secret) {
		t.Fatalf("missing required secret not blamed: %v", causes(e))
	}
}

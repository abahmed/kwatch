package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// TestErrorClassProgramDefectMisfits: a defect in the program's own code
// is not what a configuration, a policy, a permission or a node causes.
// It is a penalty, not a veto: an edit may still trigger one.
func TestErrorClassProgramDefectMisfits(t *testing.T) {
	v := newView(newFixture(t).snapshot())
	for _, kind := range []inventory.Kind{
		kube.KindConfigMap, kube.KindSecret, kube.KindNetworkPolicy,
		"role", "rolebinding", "clusterrole", "clusterrolebinding",
		kube.KindNode,
	} {
		t.Run(string(kind), func(t *testing.T) {
			c := &candidate{id: inventory.CoreID(kind, "shop", "obj")}
			o := v.classOutcome(programDefect, c)
			requireWeight(t, o, -ErrorClassPenalty)
			if o.veto != "" {
				t.Fatalf("a misfit must not veto: %q", o.veto)
			}
		})
	}
	c := &candidate{id: inventory.CoreID(kube.KindDeployment, "shop", "api")}
	requireWeight(t, v.classOutcome(programDefect, c), ErrorClassWeight)
}

// changedSecretCase fails two pods that read a Secret edited three
// minutes before with text; it returns the Secret and a failing pod.
func changedSecretCase(
	f *fixture, field, text string,
) (inventory.EntityID, inventory.EntityID) {
	pods := f.workload("shop", "api", 2)
	secret := usesSecret(f, "tls-cert", pods...)
	f.change(secret, 1, field)
	for _, pod := range pods {
		f.fail(containerOf(pod), "CrashLoop", failingH, 4, text)
	}
	return secret, containerOf(pods[0])
}

// TestChangeOnlyDependencyIsCappedForProgramDefects: a Secret that only
// changed shortly before failures that read as a defect of the program
// is a suspect at most; the workload stays the root.
func TestChangeOnlyDependencyIsCappedForProgramDefects(t *testing.T) {
	f := newFixture(t)
	secret, effect := changedSecretCase(f, "data.tls.crt", rcaPanic)
	e := f.explain()
	if blames(e, secret) {
		t.Fatalf("a bare edit was stated as the cause: %v", causes(e))
	}
	requireCause(t, e, effect, "deployment/shop/api")
	for _, area := range e.Areas {
		for _, c := range area.Trace.Candidates {
			if c.Root == secret && c.Confidence > ChangeOnlyMaxConfidence {
				t.Fatalf("confidence %.2f is above the cap", c.Confidence)
			}
		}
	}
}

// TestChangeOnlyCapNeedsAProgramDefect: the same edit before failures
// that read as a bad configuration value keeps its confidence.
func TestChangeOnlyCapNeedsAProgramDefect(t *testing.T) {
	f := newFixture(t)
	secret, effect := changedSecretCase(f, "data.tls.crt",
		"panic: strconv.ParseInt: parsing \"0.14\": invalid syntax")
	c := requireCause(t, f.explain(), effect, secret.String())
	if c.Confidence <= ChangeOnlyMaxConfidence {
		t.Fatalf("confidence %.2f was capped", c.Confidence)
	}
}

// TestChangeOnlyCapYieldsToTheErrorNamingIt: a defect whose words name
// the changed key is tied to the edit, so the cap does not apply.
func TestChangeOnlyCapYieldsToTheErrorNamingIt(t *testing.T) {
	f := newFixture(t)
	secret, effect := changedSecretCase(f, "data.tls-bundle",
		"panic: runtime error: nil pointer dereference reading "+
			"tls-bundle")
	c := requireCause(t, f.explain(), effect, secret.String())
	if c.Confidence <= ChangeOnlyMaxConfidence {
		t.Fatalf("confidence %.2f was capped", c.Confidence)
	}
}

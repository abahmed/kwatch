package reason

import (
	"testing"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// pulling adds a workload whose pod container pulls image and fails.
func pulling(h *harness, name, image, summary string) signal.Signal {
	owner := eid(kube.KindReplicaSet, "ns", name)
	p := podID(name + "-pod")
	c := eid(kube.KindContainer, "ns", name+"-c")
	h.ownedPod(p, owner)
	h.observe(c, nil)
	h.relate(c, knowledge.PartOf, p)
	h.relate(c, knowledge.Pulls, eid(kube.KindImage, "", image))
	return h.evidenceSignal(c, constant.ReasonImagePullBackOff, summary,
		t0)
}

func TestRegistryRuleBlamesSharedRegistry(t *testing.T) {
	h := newHarness(t, t0)
	sym := pulling(h, "a", "reg.io/a:1", "pull i/o timeout")
	pulling(h, "b", "reg.io/b:1", "pull failed")

	hyps := RegistryRule{}.Explain(h.query(), sym)
	root := eid(KindRegistry, "", "reg.io")
	if len(hyps) != 1 || hyps[0].Root != root {
		t.Fatalf("hyps = %+v", hyps)
	}
	if hyps[0].Score < High {
		t.Errorf("registry error should raise score: %v", hyps[0].Score)
	}
}

func TestRegistryRuleSingleWorkloadIsNotARegistryOutage(t *testing.T) {
	h := newHarness(t, t0)
	sym := pulling(h, "a", "reg.io/a:1", "not found")
	pulling(h, "b", "other.io/b:1", "not found")

	if hyps := (RegistryRule{}).Explain(h.query(), sym); hyps != nil {
		t.Errorf("hyps = %+v", hyps)
	}
}

func TestRegistryRuleIgnoresOtherSymptoms(t *testing.T) {
	h := newHarness(t, t0)
	sym := h.signal(podID("p"), "Failed", t0, 0)
	if hyps := (RegistryRule{}).Explain(h.query(), sym); hyps != nil {
		t.Errorf("hyps = %+v", hyps)
	}
	noImage := h.signal(podID("q"), constant.ReasonErrImagePull, t0, 0)
	if hyps := (RegistryRule{}).Explain(h.query(), noImage); hyps != nil {
		t.Errorf("no image: %+v", hyps)
	}
}

func TestRegistryHostDefaultsToDockerHub(t *testing.T) {
	cases := map[string]string{
		"nginx":             "docker.io",
		"library/nginx":     "docker.io",
		"ghcr.io/org/app:1": "ghcr.io",
		"localhost/app":     "localhost",
		"registry:5000/app": "registry:5000",
		"myorg/app":         "docker.io",
	}
	for image, want := range cases {
		if got := RegistryHost(image); got != want {
			t.Errorf("RegistryHost(%q) = %q want %q", image, got, want)
		}
	}
}

func TestRegistryErrorRecognisesTransportFailures(t *testing.T) {
	if !registryError("TooManyRequests: rate limit") {
		t.Error("rate limit is a registry error")
	}
	if registryError("manifest unknown") {
		t.Error("a missing tag is not a registry outage")
	}
}

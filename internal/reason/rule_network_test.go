package reason

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

func text(v string) map[string]knowledge.Value {
	return map[string]knowledge.Value{kube.AttrLabels: knowledge.Text(v)}
}

func policyHarness(t *testing.T, selector string) (
	*harness, knowledge.EntityID,
) {
	h := newHarness(t, t0)
	h.observe(podID("p1"), text("app=web"))
	pol := eid(kube.KindNetworkPolicy, "ns", "deny")
	h.observe(pol, map[string]knowledge.Value{
		kube.AttrSelector: knowledge.Text(selector)})
	return h, pol
}

func TestNetworkPolicyRuleBlamesRecentSelectingPolicy(t *testing.T) {
	h, pol := policyHarness(t, "app=web")
	h.change(pol, t0.Add(-2*time.Minute), true)
	sym := h.signal(podID("p1"), "Failed", t0, signal.Critical)

	hyps := NetworkPolicyRule{}.Explain(h.query(), sym)
	if len(hyps) != 1 || hyps[0].Root != pol {
		t.Fatalf("hyps = %+v", hyps)
	}
	if hyps[0].Score < DefaultFloor || hyps[0].Change == nil ||
		!strings.Contains(hyps[0].Summary, "network policy deny") {
		t.Errorf("hyp = %+v", hyps[0])
	}
}

func TestNetworkPolicyRuleEmptySelectorSelectsEveryPod(t *testing.T) {
	for _, sel := range []string{"", "<none>"} {
		h, pol := policyHarness(t, sel)
		h.change(pol, t0.Add(-time.Minute), false, field("spec", "", "x"))
		sym := h.signal(podID("p1"), "Failed", t0, 0)
		hyps := NetworkPolicyRule{}.Explain(h.query(), sym)
		if len(hyps) != 1 {
			t.Errorf("selector %q: hyps = %+v", sel, hyps)
		}
	}
}

func TestNetworkPolicyRuleIgnoresUnrelatedPolicies(t *testing.T) {
	h, pol := policyHarness(t, "app=db")
	h.change(pol, t0.Add(-time.Minute), true)
	sym := h.signal(podID("p1"), "Failed", t0, 0)
	if hyps := (NetworkPolicyRule{}).Explain(h.query(), sym); len(hyps) != 0 {
		t.Errorf("non-selecting policy blamed: %+v", hyps)
	}

	other := eid(kube.KindNetworkPolicy, "other", "deny")
	h.observe(other, nil)
	h.change(other, t0.Add(-time.Minute), true)
	if hyps := (NetworkPolicyRule{}).Explain(h.query(), sym); len(hyps) != 0 {
		t.Errorf("other namespace blamed: %+v", hyps)
	}
}

func TestNetworkPolicyRuleWindowAndMissingChange(t *testing.T) {
	h, pol := policyHarness(t, "app=web")
	sym := h.signal(podID("p1"), "Failed", t0, 0)
	if hyps := (NetworkPolicyRule{}).Explain(h.query(), sym); len(hyps) != 0 {
		t.Errorf("no change, got %+v", hyps)
	}
	h.change(pol, t0.Add(-PolicyWindow-time.Minute), true)
	if hyps := (NetworkPolicyRule{}).Explain(h.query(), sym); len(hyps) != 0 {
		t.Errorf("stale change blamed: %+v", hyps)
	}
	h.change(pol, t0.Add(time.Minute), false, field("spec", "", "x"))
	if hyps := (NetworkPolicyRule{}).Explain(h.query(), sym); len(hyps) != 0 {
		t.Errorf("later change blamed: %+v", hyps)
	}
}

func TestNetworkPolicyRuleSkipsUnusableSymptoms(t *testing.T) {
	h, _ := policyHarness(t, "app=web")
	node := h.signal(eid(kube.KindNode, "", "n"), "x", t0, 0)
	if hyps := (NetworkPolicyRule{}).Explain(h.query(), node); hyps != nil {
		t.Errorf("node symptom: %+v", hyps)
	}
	unknown := h.signal(podID("ghost"), "x", t0, 0)
	if hyps := (NetworkPolicyRule{}).Explain(h.query(), unknown); hyps != nil {
		t.Errorf("unobserved pod: %+v", hyps)
	}
	h.observe(podID("bad"), text("app in (a)"))
	bad := h.signal(podID("bad"), "x", t0, 0)
	if hyps := (NetworkPolicyRule{}).Explain(h.query(), bad); hyps != nil {
		t.Errorf("unparsable labels: %+v", hyps)
	}
}

func TestDNSRuleBlamesClusterDNSWhenProbeFails(t *testing.T) {
	h := newHarness(t, t0)
	h.signal(kube.ClusterDNS, "DNSFailing", t0, signal.Critical)
	sym := h.evidenceSignal(podID("p1"), "Failed",
		"dial tcp: lookup db: no such host", t0)

	hyps := DNSRule{}.Explain(h.query(), sym)
	if len(hyps) != 1 || hyps[0].Root != kube.ClusterDNS {
		t.Fatalf("hyps = %+v", hyps)
	}
	if hyps[0].Score < High || hyps[0].Summary != "cluster DNS is failing" {
		t.Errorf("hyp = %+v", hyps[0])
	}
}

func TestDNSRuleHealthyProbeContradicts(t *testing.T) {
	h := newHarness(t, t0)
	sym := h.evidenceSignal(podID("p1"), "Failed",
		"Temporary failure in name resolution", t0)

	hyps := DNSRule{}.Explain(h.query(), sym)
	if len(hyps) != 1 || hyps[0].Score >= DefaultFloor {
		t.Fatalf("hyps = %+v", hyps)
	}
	if hyps[0].Summary != "name resolution fails" {
		t.Errorf("summary = %q", hyps[0].Summary)
	}
}

func TestDNSRuleIgnoresOtherErrors(t *testing.T) {
	h := newHarness(t, t0)
	sym := h.evidenceSignal(podID("p1"), "Failed", "connection refused", t0)
	if hyps := (DNSRule{}).Explain(h.query(), sym); hyps != nil {
		t.Errorf("hyps = %+v", hyps)
	}
}

func TestMetricsAPIRuleBlamesUnavailableAPI(t *testing.T) {
	h := newHarness(t, t0)
	api := eid("apiservice", "", "v1beta1.metrics.k8s.io")
	h.signal(api, "Unavailable", t0, signal.Critical)
	hpa := eid(kube.KindHPA, "ns", "web")
	sym := h.evidenceSignal(hpa, constant.ReasonFailedGetResourceMetric,
		"unable to fetch metrics from metrics.k8s.io", t0)

	hyps := MetricsAPIRule{}.Explain(h.query(), sym)
	if len(hyps) != 1 || hyps[0].Root != api {
		t.Fatalf("hyps = %+v", hyps)
	}
	if hyps[0].Score < High {
		t.Errorf("score = %v", hyps[0].Score)
	}
}

func TestMetricsAPIRuleNegativeCases(t *testing.T) {
	h := newHarness(t, t0)
	hpa := eid(kube.KindHPA, "ns", "web")
	sym := h.signal(hpa, constant.ReasonFailedGetResourceMetric, t0, 0)
	if hyps := (MetricsAPIRule{}).Explain(h.query(), sym); len(hyps) != 0 {
		t.Errorf("available API blamed: %+v", hyps)
	}
	other := h.signal(hpa, "Other", t0, 0)
	if hyps := (MetricsAPIRule{}).Explain(h.query(), other); hyps != nil {
		t.Errorf("other reason: %+v", hyps)
	}
}

func TestParseSchedulerMessageSortsBlockers(t *testing.T) {
	msg := "0/5 nodes are available: 2 node(s) had untolerated taint " +
		"{a: b}, 3 Insufficient memory."
	blockers, total := ParseSchedulerMessage(msg)
	if total != 5 || len(blockers) != 2 {
		t.Fatalf("total=%d blockers=%v", total, blockers)
	}
	if blockers[0].Reason != "Insufficient memory" || blockers[0].Nodes != 3 {
		t.Errorf("top = %+v", blockers[0])
	}
	if blockers[1].Reason != "had untolerated taint" {
		t.Errorf("second = %+v", blockers[1])
	}
	if b, n := ParseSchedulerMessage("no colon"); b != nil || n != 0 {
		t.Error("message without a colon is not a scheduler message")
	}
}

func TestSchedulingRuleNamesDominantBlocker(t *testing.T) {
	h := newHarness(t, t0)
	sym := h.evidenceSignal(podID("p1"), constant.ReasonUnschedulable,
		"unschedulable", t0, signal.Evidence{Label: "scheduler",
			Value: "0/4 nodes are available: 4 Insufficient cpu."})

	hyps := SchedulingRule{}.Explain(h.query(), sym)
	if len(hyps) != 1 || hyps[0].Root.Kind != KindScheduling {
		t.Fatalf("hyps = %+v", hyps)
	}
	if hyps[0].Summary != "Insufficient cpu on 4 of 4 nodes" ||
		hyps[0].Score < High {
		t.Errorf("hyp = %+v", hyps[0])
	}
}

func TestSchedulingRuleNeedsSchedulerEvidence(t *testing.T) {
	h := newHarness(t, t0)
	sym := h.signal(podID("p1"), constant.ReasonUnschedulable, t0, 0)
	if hyps := (SchedulingRule{}).Explain(h.query(), sym); hyps != nil {
		t.Errorf("hyps = %+v", hyps)
	}
	other := h.signal(podID("p1"), "Failed", t0, 0)
	if hyps := (SchedulingRule{}).Explain(h.query(), other); hyps != nil {
		t.Errorf("hyps = %+v", hyps)
	}
}

func TestAdmissionRuleBlamesNamedFailingWebhook(t *testing.T) {
	h := newHarness(t, t0)
	hook := eid(kube.KindValidatingHook, "", "policy-hook")
	h.observe(hook, nil)
	h.signal(hook, "Down", t0, signal.Critical)
	h.signals[hook][0].Summary = "Endpoints unavailable"
	sym := h.evidenceSignal(eid(kube.KindReplicaSet, "ns", "rs"),
		"FailedCreate",
		`admission webhook "policy-hook" denied the request`, t0)

	hyps := AdmissionRule{}.Explain(h.query(), sym)
	if len(hyps) != 1 || hyps[0].Root != hook {
		t.Fatalf("hyps = %+v", hyps)
	}
	want := "admission webhook policy-hook: endpoints unavailable"
	if hyps[0].Summary != want || hyps[0].Score < High {
		t.Errorf("hyp = %+v", hyps[0])
	}
}

func TestAdmissionRuleWithoutHookSignalStillNamesWebhook(t *testing.T) {
	h := newHarness(t, t0)
	hook := eid(kube.KindMutatingWebhook, "", "injector")
	h.observe(hook, nil)
	sym := h.evidenceSignal(podID("p1"), "Failed",
		"webhook injector timed out", t0)

	hyps := AdmissionRule{}.Explain(h.query(), sym)
	if len(hyps) != 1 ||
		hyps[0].Summary != "admission webhook injector rejects requests" {
		t.Fatalf("hyps = %+v", hyps)
	}
}

func TestAdmissionRuleNegativeCases(t *testing.T) {
	h := newHarness(t, t0)
	h.observe(eid(kube.KindValidatingHook, "", "other"), nil)
	plain := h.signal(podID("p1"), "Failed", t0, 0)
	if hyps := (AdmissionRule{}).Explain(h.query(), plain); hyps != nil {
		t.Errorf("no webhook text: %+v", hyps)
	}
	sym := h.evidenceSignal(podID("p2"), "Failed", "webhook foo denied", t0)
	if hyps := (AdmissionRule{}).Explain(h.query(), sym); len(hyps) != 0 {
		t.Errorf("unnamed webhook blamed: %+v", hyps)
	}
}

func TestQuotaRuleBlamesNamespaceQuota(t *testing.T) {
	h := newHarness(t, t0)
	quota := eid(kube.KindQuota, "ns", "compute")
	h.observe(quota, nil)
	h.observe(eid(kube.KindQuota, "other", "compute"), nil)
	h.signal(quota, "Exhausted", t0, signal.Warning)
	sym := h.evidenceSignal(eid(kube.KindReplicaSet, "ns", "rs"),
		"FailedCreate", "pods forbidden: exceeded quota: compute", t0)

	hyps := QuotaRule{}.Explain(h.query(), sym)
	if len(hyps) != 1 || hyps[0].Root != quota {
		t.Fatalf("hyps = %+v", hyps)
	}
	if hyps[0].Score < High ||
		hyps[0].Summary != "resource quota compute is used up" {
		t.Errorf("hyp = %+v", hyps[0])
	}
}

func TestQuotaRuleNeedsQuotaErrorText(t *testing.T) {
	h := newHarness(t, t0)
	h.observe(eid(kube.KindQuota, "ns", "compute"), nil)
	sym := h.signal(eid(kube.KindReplicaSet, "ns", "rs"), "x", t0, 0)
	if hyps := (QuotaRule{}).Explain(h.query(), sym); hyps != nil {
		t.Errorf("hyps = %+v", hyps)
	}
}

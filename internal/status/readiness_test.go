package status

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func readinessFindings() []detection.Finding {
	return []detection.Finding{
		{Entity: inventory.CoreID(kube.KindDeprecatedAPI, "",
			"batch/v1beta1 cronjobs"),
			Reason: reasons.DeprecatedAPIInUse, Summary: "removed in 1.25"},
		{Entity: inventory.CoreID(kube.KindValidatingHook, "",
			"policy.example.com"), Reason: reasons.WebhookNoEndpoints,
			Severity: detection.Critical},
		// failurePolicy Ignore: requests skip it, so it blocks nothing.
		{Entity: inventory.CoreID(kube.KindValidatingHook, "", "lenient"),
			Reason: reasons.WebhookNoEndpoints, Severity: detection.Info},
		// A risk is not a blocker.
		{Entity: inventory.EntityID{Kind: kube.KindDeployment,
			Namespace: "shop", Name: "api"},
			Reason: reasons.RiskSingleReplica, Advisory: true},
	}
}

func TestReadinessNamesOnlyWhatIsTrueNow(t *testing.T) {
	m := testModel(t)
	pdb := inventory.EntityID{Kind: kube.KindPDB, Namespace: "shop",
		Name: "api-pdb"}
	observe(t, m, pdb, map[string]inventory.Value{
		kube.AttrDisruptionsAllowed: inventory.Number(0),
		kube.AttrExpectedPods:       inventory.Number(2)})
	idle := inventory.EntityID{Kind: kube.KindPDB, Namespace: "shop",
		Name: "idle"}
	observe(t, m, idle, map[string]inventory.Value{
		kube.AttrDisruptionsAllowed: inventory.Number(0),
		kube.AttrExpectedPods:       inventory.Number(0)})

	got := Assess(readinessFindings(), m)

	want := "Upgrade readiness: 3 blockers — " +
		"batch/v1beta1 cronjobs is still requested (removed in 1.25); " +
		"PDB shop/api-pdb currently allows 0 disruptions; " +
		"webhook policy.example.com (Fail) has no ready endpoints."
	if line := got.Line(); line != want {
		t.Fatalf("line = %q\nwant   %q", line, want)
	}
	if strings.Contains(got.Line(), "single") {
		t.Fatal("a configuration risk is not a blocker")
	}
}

func TestReadinessBoundsEachClass(t *testing.T) {
	var findings []detection.Finding
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		findings = append(findings, detection.Finding{
			Entity: inventory.CoreID(kube.KindNode, "", n),
			Reason: reasons.ClusterVersionSkew, Summary: "Kubelet old"})
	}
	got := Assess(findings, nil)
	if got.Total != 5 || len(got.Items) != 4 {
		t.Fatalf("total %d items %d", got.Total, len(got.Items))
	}
	if last := got.Items[3].Text; last !=
		"and 2 more nodes outside the version skew" {
		t.Fatalf("last = %q", last)
	}
}

func TestReadinessSkipsListedEntitiesAndKeysChanges(t *testing.T) {
	got := Assess(readinessFindings(), nil)
	skip := map[inventory.EntityID]bool{
		inventory.CoreID(kube.KindDeprecatedAPI, "",
			"batch/v1beta1 cronjobs"): true}
	rest := got.Skipping(skip)
	if rest.Total != 1 || rest.Key() == got.Key() {
		t.Fatalf("rest = %+v", rest)
	}
	if Assess(nil, nil).Line() != "" {
		t.Fatal("no blockers, no line")
	}
}

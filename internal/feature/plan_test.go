package feature

import (
	"strings"
	"testing"
	"time"
)

func TestCommunityPlanHonorsOverridesAndDependencies(t *testing.T) {
	plan, err := Build(
		CommunityPolicy(),
		map[ID]bool{DirectDiagnosis: true, DependencyGraph: true, ImpactAnalysis: true, RCAFeedback: true},
		Overrides{Disabled: []ID{DependencyGraph}},
		time.Unix(0, 0),
	)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !plan.Enabled(DirectDiagnosis) {
		t.Fatal("direct diagnosis should remain independent")
	}
	if plan.Enabled(DependencyGraph) || plan.Enabled(ImpactAnalysis) {
		t.Fatal("disabled dependency should disable its dependent capabilities")
	}
	if !strings.Contains(plan.Decisions[ImpactAnalysis].Reason, string(DependencyGraph)) {
		t.Fatalf("impact reason = %q", plan.Decisions[ImpactAnalysis].Reason)
	}
}

func TestCommunityPlanDoesNotGrantPro(t *testing.T) {
	plan, err := Build(
		CommunityPolicy(),
		map[ID]bool{DirectDiagnosis: true, RCAFeedback: true, AdvancedRouting: true},
		Overrides{},
		time.Unix(0, 0),
	)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !plan.Enabled(RCAFeedback) {
		t.Fatal("community RCA feedback should be enabled")
	}
	if plan.Enabled(AdvancedRouting) {
		t.Fatal("community policy must not grant Pro capabilities")
	}
}

func TestBuildRejectsUnknownOverride(t *testing.T) {
	_, err := Build(CommunityPolicy(), nil, Overrides{Disabled: []ID{"not-a-feature"}}, time.Now())
	if err == nil {
		t.Fatal("Build() accepted an unknown feature ID")
	}
}

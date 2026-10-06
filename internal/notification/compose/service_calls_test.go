package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

var callNow = time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)

func TestCalledServiceIsTheRootOrNamedByTheMode(t *testing.T) {
	service := inventory.CoreID(kube.KindService, "shop", "redis")
	workload := inventory.CoreID(kube.KindDeployment, "shop", "redis")

	got, ok := calledService(&rootcause.CauseRecord{
		Rule: "called-service-no-endpoints", Root: service})
	if !ok || got != service {
		t.Fatalf("service root: got %v %v", got, ok)
	}
	got, ok = calledService(&rootcause.CauseRecord{
		Rule: "called-service-backends-failing", Root: workload,
		Mode: "BackendsFailing.shop/redis"})
	if !ok || got != service {
		t.Fatalf("workload root: got %v %v", got, ok)
	}
	if _, ok := calledService(&rootcause.CauseRecord{Rule: "self",
		Root: workload}); ok {
		t.Fatal("a self cause names no Service")
	}
}

func TestLeadFailurePrefersWhatRunsUnderTheRoot(t *testing.T) {
	root := inventory.CoreID(kube.KindDeployment, "shop", "redis")
	api := detection.Finding{Entity: inventory.CoreID(kube.KindContainer,
		"shop", "api-1-a/app")}
	cache := detection.Finding{Entity: inventory.CoreID(kube.KindContainer,
		"shop", "redis-1-a/app")}

	if got := leadFailure(root, []detection.Finding{api, cache}); got.Entity !=
		cache.Entity {
		t.Fatalf("lead = %v, want the cache's own failure", got.Entity)
	}
	if got := leadFailure(root, []detection.Finding{api}); got.Entity !=
		api.Entity {
		t.Fatalf("lead = %v, want the first failure", got.Entity)
	}
}

func TestCalledServiceSentenceQuotesTheCallersLine(t *testing.T) {
	service := inventory.CoreID(kube.KindService, "shop", "redis")
	api := inventory.CoreID(kube.KindDeployment, "shop", "api")
	crash := inventory.CoreID(kube.KindContainer, "shop", "api-1-a/app")
	line := "dial tcp redis:6379: connect: connection refused"
	p := incident.Incident{
		Root: service, Impact: []inventory.EntityID{api},
		Cause: &rootcause.CauseRecord{Rule: "called-service-no-endpoints",
			Root: service},
		Members: map[detection.Key]detection.Finding{
			{Entity: crash, Reason: "CrashLoopBackOff"}: {Entity: crash,
				Reason: "CrashLoopBackOff", Severity: detection.Critical,
				Evidence: []detection.Evidence{{
					Label: detection.EvidenceError, Value: line}}},
			{Entity: service, Reason: "ServiceNoEndpoints"}: {Entity: service,
				Reason: "ServiceNoEndpoints", Mode: detection.ModeNoEndpoints,
				Since: callNow.Add(-2 * time.Minute)},
		},
	}
	p.Cause.Root = inventory.CoreID(kube.KindDeployment, "shop", "redis")
	p.Cause.Rule = "called-service-backends-failing"
	p.Cause.Mode = "BackendsFailing.shop/redis"

	got := calledServiceSentences(gatherFacts(incident.Decision{Incident: p},
		callNow, nil))

	if len(got) != 1 || !strings.Contains(got[0].text,
		`api fails calling service redis with "`+line+`"`) ||
		!strings.Contains(got[0].text, "no ready endpoints since 09:58") {
		t.Fatalf("sentences = %+v", got)
	}
}

func TestPolicyBlockLeadNamesTheCallAndTheChange(t *testing.T) {
	policy := inventory.CoreID(kube.KindNetworkPolicy, "shop", "deny-all")
	api := inventory.CoreID(kube.KindDeployment, "shop", "api")
	crash := inventory.CoreID(kube.KindContainer, "shop", "api-1-a/app")
	p := incident.Incident{
		Root: policy, Impact: []inventory.EntityID{api},
		Cause: &rootcause.CauseRecord{Rule: "policy-blocks-call",
			Root: policy, Mode: "BlocksCall.postgres:5432",
			Change: &inventory.Change{Entity: policy, Created: true,
				At: callNow, Actor: "bob"}},
		Members: map[detection.Key]detection.Finding{
			{Entity: crash, Reason: "CrashLoopBackOff"}: {Entity: crash,
				Reason: "CrashLoopBackOff", Severity: detection.Critical},
		},
	}

	got := leadText(gatherFacts(incident.Decision{Incident: p}, callNow,
		nil))

	for _, want := range []string{"can't reach postgres:5432",
		"network policy deny-all (created 10:00 by bob) blocks the call"} {
		if !strings.Contains(got, want) {
			t.Fatalf("lead = %q, want %q", got, want)
		}
	}
}

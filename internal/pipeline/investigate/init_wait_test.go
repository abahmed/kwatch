package investigate

import (
	"context"
	"testing"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func initWaitSources(
	t *testing.T, calls string, lines []string,
) (Sources, incident.Incident) {
	container := kube.ContainerID("shop", "api-1", "wait-for-db")
	attrs := map[string]inventory.Value{}
	if calls != "" {
		attrs[kube.AttrServiceCalls] = text(calls)
	}
	db := inventory.CoreID(kube.KindService, "shop", "db")
	slice := inventory.CoreID(kube.KindEndpointSlice, "shop", "db-1")
	model := testModel(t,
		observed(container, attrs),
		observed(db, map[string]inventory.Value{}),
		observed(slice, map[string]inventory.Value{
			kube.AttrEndpointsReady: num(0)}),
		related(slice, inventory.Backs, db))
	read := func(context.Context, inventory.EntityID) []string {
		return lines
	}
	p := incidentOf(db, finding(container, "InitWaiting"))
	return Sources{Model: model, CurrentLogs: read}, p
}

func TestInvestigateInitWaitQuotesTheLastLines(t *testing.T) {
	sources, p := initWaitSources(t, "shop/db:5432", []string{
		"waiting for db:5432...", "waiting for db:5432...",
		"waiting for db:5432..."})

	plan, r := planAndRun(t, sources, p)

	if plan.Kind != kindInitWait {
		t.Fatalf("kind = %q, want init-wait", plan.Kind)
	}
	if len(r.Output) != 1 || r.Output[0] != "waiting for db:5432..." {
		t.Fatalf("output = %q, want the repeated line once", r.Output)
	}
	if got := evidenceOf(r, incident.FactDependency); len(got) != 0 {
		t.Fatalf("dependency = %+v: the finding already names it", got)
	}
}

func TestInvestigateInitWaitNamesTheServiceTheOutputNames(t *testing.T) {
	sources, p := initWaitSources(t, "", []string{
		"checking...", "waiting for db:5432..."})

	_, r := planAndRun(t, sources, p)

	got := evidenceOf(r, incident.FactDependency)
	want := "Service db in shop, which has no ready endpoints"
	if len(got) != 1 || got[0].Text != want {
		t.Fatalf("dependency = %+v, want %q", got, want)
	}
	if len(r.Output) != 2 {
		t.Fatalf("output = %q, want the last two lines", r.Output)
	}
}

func TestInvestigateInitWaitIgnoresUnknownHosts(t *testing.T) {
	sources, p := initWaitSources(t, "", []string{
		"waiting for cache:6379...", "contacting 10.0.0.4:5432"})

	_, r := planAndRun(t, sources, p)

	if got := evidenceOf(r, incident.FactDependency); len(got) != 0 {
		t.Fatalf("dependency = %+v, want none for unknown hosts", got)
	}
}

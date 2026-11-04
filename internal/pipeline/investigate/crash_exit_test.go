package investigate

import (
	"context"
	"testing"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// redisCrashLine is the first line a .NET host printed before it died
// with exit 139 on a Redis it could not reach.
const redisCrashLine = "Unhandled exception. StackExchange.Redis." +
	"RedisConnectionException: It was not possible to connect to the " +
	"redis server(s). Error connecting right now."

// crashedIncident is a flapping single-replica Deployment whose container
// exited 139 and is killed by liveness while its Service has no
// endpoint: the modes kwatch saw on the real incident.
func crashedIncident(t *testing.T) (*inventory.Model, incident.Incident,
	inventory.EntityID) {
	t.Helper()
	root := inventory.CoreID(kube.KindDeployment, "web", "api")
	pod := inventory.CoreID(kube.KindPod, "web", "api-6d9f-x1")
	app := kube.ContainerID("web", "api-6d9f-x1", "api")
	svc := inventory.CoreID(kube.KindService, "web", "api")
	model := testModel(t,
		observed(pod, nil),
		observed(app, map[string]inventory.Value{
			kube.AttrRestarts:     num(3),
			kube.AttrLastExitCode: num(139),
			kube.AttrLastReason:   text("Error")}),
		related(app, inventory.PartOf, pod),
		observed(svc, nil))
	p := incidentOf(root, finding(app, "Exit.Segfault"),
		finding(pod, "Probe.Liveness"), finding(svc, "NoEndpoints"))
	p.State = incident.Flapping
	return model, p, app
}

func TestInvestigateCrashReadsErrorExitOfFlappingIncident(t *testing.T) {
	model, p, app := crashedIncident(t)
	var read []inventory.EntityID
	logs := func(_ context.Context, id inventory.EntityID) []string {
		read = append(read, id)
		return []string{"info: starting", redisCrashLine,
			"at Api.Cache.CacheClient..ctor()"}
	}

	plan, r := planAndRun(t, Sources{Model: model, Logs: logs}, p)

	if plan.Kind != incident.CheckCrash {
		t.Fatalf("kind = %q, want the crash check to lead", plan.Kind)
	}
	if len(read) != 1 || read[0] != app {
		t.Fatalf("read = %v, want the previous log of %v", read, app)
	}
	errs := evidenceOf(r, incident.FactError)
	if len(errs) != 1 || errs[0].Text != redisCrashLine {
		t.Fatalf("errors = %+v, want the Redis line", errs)
	}
	if len(r.Output) == 0 {
		t.Fatal("no output captured for the crashed container")
	}
}

func TestInvestigateCrashReadsErroredContainerOfMemberPod(t *testing.T) {
	model, p, app := crashedIncident(t)
	for key, m := range p.Members {
		if key.Entity == app {
			delete(p.Members, key)
		} else if m.Mode == "Probe.Liveness" {
			m.Mode, m.Reason = "Probe.Readiness", "Probe.Readiness"
			p.Members[key] = m
		}
	}
	var read []inventory.EntityID
	logs := func(_ context.Context, id inventory.EntityID) []string {
		read = append(read, id)
		return []string{redisCrashLine}
	}

	plan, _ := planAndRun(t, Sources{Model: model, Logs: logs}, p)

	if plan.Kind != incident.CheckCrash || len(read) != 1 ||
		read[0] != app {
		t.Fatalf("kind = %q, read = %v: want the pod's errored "+
			"container read", plan.Kind, read)
	}
}

func TestInvestigateCrashSkipsCleanExit(t *testing.T) {
	_, p, app := crashedIncident(t)
	for key := range p.Members {
		if key.Entity == app {
			delete(p.Members, key)
		}
	}
	model := testModel(t, observed(app, map[string]inventory.Value{
		kube.AttrRestarts: num(3), kube.AttrLastExitCode: num(0)}))

	if hasErrorExit(p, model) {
		t.Fatal("a clean exit must not lead the crash check")
	}
}

// Any non-zero exit leads the crash check, whatever the runtime or exit
// family: a Go program's exit 2, a JVM's exit 1, an abort's 134.
func TestInvestigateCrashLeadsForAnyErrorExit(t *testing.T) {
	for _, code := range []float64{1, 2, 134, 137, 255} {
		_, p, app := crashedIncident(t)
		model := testModel(t, observed(app, map[string]inventory.Value{
			kube.AttrRestarts: num(1), kube.AttrLastExitCode: num(code)}))
		for key, m := range p.Members {
			if key.Entity == app {
				m.Mode, m.Reason = "HighRestartCount", "HighRestartCount"
				p.Members[key] = m
			}
		}
		if !crashCheck().leads(p, model) {
			t.Errorf("exit %v: the crash check must lead", code)
		}
	}
}

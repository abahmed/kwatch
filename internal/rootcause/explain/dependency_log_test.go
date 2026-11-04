package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Lines several runtimes print when a dependency cannot be reached.
const (
	dotnetCacheLine = "Unhandled exception. StackExchange.Redis." +
		"RedisConnectionException: It was not possible to connect to " +
		"the redis server(s). Error connecting right now."
	jdbcLine = "org.postgresql.util.PSQLException: Connection to " +
		"db:5432 refused. Check that the hostname and port are correct."
	goNetLine = "failed to start: dial tcp cache.data:6379: " +
		"connect: connection refused"
	externalLine = "failed to start: dial tcp db.example.com:5432: " +
		"connect: connection refused"
)

// crashOn makes one replica of workload crash with line as its own
// error.
func crashOn(f *fixture, namespace, name, line string) inventory.EntityID {
	pods := f.workload(namespace, name, 1)
	id := containerOf(pods[0])
	f.fail(id, detection.ModeCrashLoop, failingH, 2, line)
	return id
}

var dependencyLogRowCases = []rowCase{
	{row: DependencyLogRow, want: "deployment/shop/api",
		build: func(f *fixture) inventory.EntityID {
			return crashOn(f, "shop", "api", dotnetCacheLine)
		}},
}

func TestDependencyInReadsClientsOfSeveralStacks(t *testing.T) {
	cases := map[string]Dependency{
		dotnetCacheLine: {Name: "Redis"},
		jdbcLine:        {Name: "PostgreSQL", Host: "db:5432"},
		goNetLine:       {Name: "Redis", Host: "cache.data:6379"},
		externalLine:    {Name: "PostgreSQL", Host: "db.example.com:5432"},
		"Error: connect ECONNREFUSED 10.0.0.12:6379": {Name: "Redis",
			Host: "10.0.0.12:6379"},
		"pymongo.errors.ServerSelectionTimeoutError: mongo:27017: " +
			"[Errno 111] Connection refused": {Name: "MongoDB",
			Host: "mongo:27017"},
		"com.mysql.cj.jdbc.exceptions.CommunicationsException: " +
			"Communications link failure": {Name: "MySQL"},
		"could not connect to queue.internal.example: timed out": {
			Host: "queue.internal.example"},
	}
	for line, want := range cases {
		got, ok := DependencyIn(line)
		if !ok || got != want {
			t.Errorf("%q: got %+v (%v), want %+v", line, got, ok, want)
		}
	}
}

func TestDependencyInNeedsAFailedConnectionAndATarget(t *testing.T) {
	for _, line := range []string{
		"listening on :8080",
		"panic: runtime error: invalid memory address",
		"connection refused",
		"redis: cache warmed in 30ms",
		"at com.shop.Main.main(Main.java:12) timed out",
	} {
		if d, ok := DependencyIn(line); ok {
			t.Errorf("%q read as %+v", line, d)
		}
	}
}

// Several stacks name their dependency; each crash is blamed on the
// workload at likely confidence, never high: it is text.
func TestDependencyLogCauseIsLikelyNotHigh(t *testing.T) {
	for _, line := range []string{dotnetCacheLine, jdbcLine, goNetLine} {
		f := newFixture(t)
		readyService(f, "shop", "db")
		readyService(f, "data", "cache")
		id := crashOn(f, "shop", "api", line)
		c := requireCause(t, f.explain(), id, "deployment/shop/api")
		if c.Row != DependencyLogRow {
			t.Fatalf("%q: row = %s", line, c.Row)
		}
		if c.Confidence < ConfidenceLikely ||
			c.Confidence >= ConfidenceHigh {
			t.Errorf("%q: confidence = %.2f, want likely", line,
				c.Confidence)
		}
	}
}

// A healthy pod's log line about a dependency is no cause: the row
// needs the unit to crash or exit.
func TestDependencyLogNeedsACrash(t *testing.T) {
	f := newFixture(t)
	pods := f.workload("shop", "api", 1)
	id := containerOf(pods[0])
	f.fail(id, detection.ModeProbeReadiness, failingH, 2, dotnetCacheLine)
	if c, ok := f.explain().CauseOf(id); ok && c.Row == DependencyLogRow {
		t.Fatalf("a probe failure was blamed on the log: %v", c.Root)
	}
}

// A kubelet probe message names the pod's own port, never a call the
// program makes.
func TestDependencyLogIgnoresProbeMessages(t *testing.T) {
	f := newFixture(t)
	pods := f.workload("shop", "api", 1)
	id := containerOf(pods[0])
	f.fail(id, detection.ModeCrashLoop, failingH, 2, "")
	f.findings[id][0].Evidence = []detection.Evidence{{
		Label: probeEvidenceLabel, Value: "Readiness probe failed: " +
			"dial tcp 10.0.0.5:5432: connect: connection refused"}}
	if c, ok := f.explain().CauseOf(id); ok && c.Row == DependencyLogRow {
		t.Fatalf("the probe message was read as a dependency")
	}
}

// An in-cluster Service with no ready endpoints is shown by structure:
// the called-service row blames it, not the log row.
func TestDependencyLogLeavesEmptyServiceToCalledRows(t *testing.T) {
	f := newFixture(t)
	_, api, _ := redisFixture(f, 0, 0)
	for _, pod := range api {
		f.fail(containerOf(pod), detection.ModeCrashLoop, failingH, 2,
			"RedisConnectionException: It was not possible to connect "+
				"to redis:6379")
	}
	c := requireCause(t, f.explain(), containerOf(api[0]),
		"service/shop/redis")
	if c.Row != "called-service-no-endpoints" {
		t.Fatalf("row = %s", c.Row)
	}
}

// A host that is a healthy Service of the cluster is named as one.
func TestDependencyLogNamesAnInClusterService(t *testing.T) {
	f := newFixture(t)
	readyService(f, "shop", "db")
	id := crashOn(f, "shop", "api", jdbcLine)
	v := newView(f.snapshot())
	modes := v.dependencyModes(id)
	if len(modes) != 1 || modes[0] != ModeDependencyService {
		t.Fatalf("modes = %v, want the Service mode", modes)
	}
}

// readyService adds a Service with one ready endpoint.
func readyService(f *fixture, namespace, name string) {
	service := inventory.CoreID(kube.KindService, namespace, name)
	slice := inventory.CoreID(kube.KindEndpointSlice, namespace, name+"-x")
	f.add(service)
	f.apply(inventory.Observation{Kind: inventory.Observed, Entity: slice,
		Attributes: map[string]inventory.Value{
			kube.AttrEndpointsReady: inventory.Number(1)}})
	f.relate(slice, inventory.Backs, service)
}

// A host outside the cluster that one workload fails on is left to the
// endpoint rows: alone, it is the workload's own problem.
func TestDependencyLogLeavesExternalHostToEndpointRows(t *testing.T) {
	f := newFixture(t)
	id := crashOn(f, "shop", "api", externalLine)
	if c, ok := f.explain().CauseOf(id); ok && c.Row == DependencyLogRow {
		t.Fatalf("an external host was blamed from text: %v", c.Root)
	}
}

// Two workloads naming one dependency are a pattern for the shared rows,
// not two dependency causes.
func TestDependencyLogLeavesSharedDependencyToSharedRows(t *testing.T) {
	f := newFixture(t)
	a := crashOn(f, "shop", "api", dotnetCacheLine)
	b := crashOn(f, "shop", "worker", dotnetCacheLine)
	e := f.explain()
	for _, id := range []inventory.EntityID{a, b} {
		if c, ok := e.CauseOf(id); ok && c.Row == DependencyLogRow {
			t.Fatalf("%v blamed on its own log", id)
		}
	}
}

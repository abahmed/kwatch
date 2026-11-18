package explain

import (
	"fmt"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

func TestKindOnlyDependencyReadsTheTables(t *testing.T) {
	cases := []struct{ text, want string }{
		{"RedisConnectionException: It was not possible to connect " +
			"to the redis server(s)", "cache (Redis)"},
		{"could not connect to the database: connection timed out",
			"database"},
		{"Npgsql.NpgsqlException: Failed to connect to server",
			"database (PostgreSQL)"},
		{"KafkaException: no brokers available", "message queue (Kafka)"},
		{"unable to connect to message broker: timed out",
			"message queue"},
		{"cache server unreachable: connection refused", "cache"},
		// An address is named: not kind-only.
		{"dial tcp 10.0.7.20:6379: connect: connection refused", ""},
		{"Failed to connect to redis.example.com:6379", ""},
		// No failure, or a kind the tables do not know: today's
		// behaviour.
		{"redis server(s) are fine", ""},
		{"could not connect to the frobnicator", ""},
	}
	for _, c := range cases {
		k, ok := KindOnlyDependency(c.text)
		if got := k.Label(); got != c.want || ok != (c.want != "") {
			t.Errorf("%q: got %q (%v), want %q", c.text, got, ok, c.want)
		}
	}
}

// Every client name the tables know has a kind: a new client row needs
// only a kind row, no logic.
func TestEveryClientHasAKind(t *testing.T) {
	for _, c := range clientNames {
		if techKinds[c.name] == "" {
			t.Errorf("client %s has no kind in techKinds", c.name)
		}
	}
	for _, name := range portNames {
		if techKinds[name] == "" {
			t.Errorf("port name %s has no kind in techKinds", name)
		}
	}
}

// kindStorm: three workloads name the cache's address, two only say
// they cannot reach the cache, the two starting after delay.
func kindStorm(
	f *fixture, kindLine string, delay time.Duration,
) (named, kindOnly []inventory.EntityID) {
	for i := range 5 {
		text, since := "dial tcp 10.0.7.20:6379: connect: connection "+
			"refused", t0.Add(2*time.Minute)
		if i >= 3 {
			text, since = kindLine, since.Add(delay)
		}
		for _, pod := range f.workload("shop", fmt.Sprintf("app%d", i), 1) {
			c := containerOf(pod)
			f.findings[c] = append(f.findings[c], detection.Finding{
				Entity: c, Reason: "CrashLoop", Mode: "CrashLoop",
				Health: failingH, Since: since, Summary: "CrashLoop",
				Evidence: []detection.Evidence{{Label: "error",
					Value: text}}})
			if i >= 3 {
				kindOnly = append(kindOnly, c)
			} else {
				named = append(named, c)
			}
		}
	}
	return named, kindOnly
}

const redisKindLine = "RedisConnectionException: It was not possible " +
	"to connect to the redis server(s)"

func TestKindOnlyFailureJoinsTheEndpointOfItsKind(t *testing.T) {
	f := newFixture(t)
	_, kindOnly := kindStorm(f, redisKindLine, time.Minute)
	e := f.explain()
	for _, id := range kindOnly {
		requireCause(t, e, id, "external-endpoint//10.0.7.20:6379")
	}
}

func TestKindOnlyFailureOfAnotherKindStaysApart(t *testing.T) {
	f := newFixture(t)
	_, kindOnly := kindStorm(f, "could not connect to the database: "+
		"connection timed out", time.Minute)
	e := f.explain()
	for _, id := range kindOnly {
		if c, ok := e.CauseOf(id); ok && c.Root.Kind == KindExternalEndpoint {
			t.Fatalf("a database failure joined the cache: %v", c.Root)
		}
	}
}

func TestKindOnlyFailureOutsideTheWindowStaysApart(t *testing.T) {
	f := newFixture(t)
	_, kindOnly := kindStorm(f, redisKindLine, SignatureWindow+time.Minute)
	e := f.explain()
	for _, id := range kindOnly {
		if c, ok := e.CauseOf(id); ok && c.Root.Kind == KindExternalEndpoint {
			t.Fatalf("a failure an hour apart joined the cache: %v", c.Root)
		}
	}
}

// Two caches failing at once: which one the kind-only failure means is
// not known, so it is attached to neither.
func TestKindOnlyFailureWithTwoCandidatesStaysApart(t *testing.T) {
	f := newFixture(t)
	_, kindOnly := kindStorm(f, redisKindLine, time.Minute)
	for i := 10; i < 13; i++ {
		for _, pod := range f.workload("shop", fmt.Sprintf("app%d", i), 1) {
			f.failError(containerOf(pod), "CrashLoop", "dial tcp "+
				"10.0.7.21:6379: connect: connection refused")
		}
	}
	e := f.explain()
	for _, id := range kindOnly {
		if c, ok := e.CauseOf(id); ok && c.Root.Kind == KindExternalEndpoint {
			t.Fatalf("attached to one of two failing caches: %v", c.Root)
		}
	}
}

// The socket error names are whole words: "FileNotFoundException" is
// not ENOTFOUND.
func TestConnectionFailureReadsWholeErrorNames(t *testing.T) {
	if ConnectionFailure("FileNotFoundException: Could not load file") {
		t.Fatal("a missing file read as a lookup failure")
	}
	for _, text := range []string{"getaddrinfo ENOTFOUND cache",
		"Error: connect ECONNREFUSED 10.0.0.5:6379",
		"lookup db on 10.96.0.10:53: i/o timeout"} {
		if !ConnectionFailure(text) {
			t.Errorf("%q not read as a connection failure", text)
		}
	}
}

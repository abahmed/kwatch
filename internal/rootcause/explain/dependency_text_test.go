package explain

import (
	"strings"
	"testing"
)

// Log lines start with a time in most formats; the time is never the
// host of the failed call.
func TestDependencyInSkipsTimestamps(t *testing.T) {
	cases := map[string]Dependency{
		"2026/11/03 12:30:45 redis: connection refused": {Name: "Redis"},
		"E1103 12:30:45.123456 1 main.go:42] redis: connection refused": {
			Name: "Redis"},
		"2026-11-03T12:30:45Z ERROR redis: connection refused": {
			Name: "Redis"},
		"2026-11-03 12:30:45,123 dial tcp cache:6379: connection refused": {
			Name: "Redis", Host: "cache:6379"},
	}
	for line, want := range cases {
		got, ok := DependencyIn(line)
		if !ok || got != want {
			t.Errorf("%q: got %+v (%v), want %+v", line, got, ok, want)
		}
	}
	for _, line := range []string{
		"2026/11/03 12:30:45 connection refused",
		"12:30:45 request timed out",
		"10:20 1234:5678 connection refused",
	} {
		if d, ok := DependencyIn(line); ok {
			t.Errorf("%q read as %+v", line, d)
		}
	}
}

// Two workloads that log a time are not sharing a dependency called
// "12".
func TestDependencyKeyIgnoresTimestamps(t *testing.T) {
	a, _ := DependencyIn("2026/11/03 12:30:45 redis: connection refused")
	b, _ := DependencyIn("2026/11/03 12:31:02 mysql: connection refused")
	if dependencyKey(a) == dependencyKey(b) {
		t.Fatalf("keys %q and %q are the same", dependencyKey(a),
			dependencyKey(b))
	}
}

// The user and password of a URL are not its host and port, and never
// reach the dependency's words.
func TestDependencyInReadsTheHostAfterUserinfo(t *testing.T) {
	cases := map[string]Dependency{
		"failed to connect to postgres://admin:12345@db:5432/app: " +
			"connection refused": {Name: "PostgreSQL", Host: "db:5432"},
		"could not connect to redis://default:98765@cache:6379 " +
			"connection refused": {Name: "Redis", Host: "cache:6379"},
		"failed to connect to postgres://admin:12345@db.prod.example." +
			"com:5432/app: connection refused": {Name: "PostgreSQL",
			Host: "db.prod.example.com:5432"},
		"Error: connect ECONNREFUSED mongodb://root:4242@mongo/admin": {
			Name: "MongoDB", Host: "mongo"},
		"amqp dial admin:s3cret@rabbit:5672: connection refused": {
			Name: "RabbitMQ", Host: "rabbit:5672"},
	}
	for line, want := range cases {
		got, ok := DependencyIn(line)
		if !ok || got != want {
			t.Errorf("%q: got %+v (%v), want %+v", line, got, ok, want)
		}
		for _, secret := range []string{"12345", "98765", "4242",
			"s3cret", "admin", "root"} {
			if strings.Contains(got.Label(), secret) {
				t.Errorf("%q: label %q has %q", line, got.Label(), secret)
			}
		}
	}
}

// A failed call to a URL with userinfo names the URL's host.
func TestEndpointInReadsTheHostAfterUserinfo(t *testing.T) {
	call, ok := endpointIn(`Get "https://bob:hunter2@api.example.com/v1":` +
		` i/o timeout`)
	if !ok || call.endpoint != "api.example.com" {
		t.Fatalf("call = %+v (%v)", call, ok)
	}
}

// A client named in another clause of the line is not the one whose
// call failed; one named in the exception class is.
func TestDependencyInTiesTheClientToTheFailure(t *testing.T) {
	for _, line := range []string{
		"request timed out after 30s; kafka consumer started",
		"connection refused\nredis cache warmed in 30ms",
		"upstream call timed out after 30s while the service was " +
			"warming up its local kafka consumer",
	} {
		if d, ok := DependencyIn(line); ok {
			t.Errorf("%q read as %+v", line, d)
		}
	}
	cases := map[string]Dependency{
		"io.lettuce.core.RedisConnectionException: the pool ran dry " +
			"while the application was busy, then the call timed out": {
			Name: "Redis"},
		"kafka: client has run out of available brokers: " +
			"connection refused": {Name: "Kafka"},
	}
	for line, want := range cases {
		got, ok := DependencyIn(line)
		if !ok || got != want {
			t.Errorf("%q: got %+v (%v), want %+v", line, got, ok, want)
		}
	}
}

// A password may hold "@" and ":"; the host is the one after the last
// "@", and no part of the password reaches the words.
func TestDependencyInReadsTheHostAfterAnAtInThePassword(t *testing.T) {
	cases := map[string]string{
		"failed to connect to postgres://app:Pa@55word@db.prod:5432/app:" +
			" connection refused": "db.prod:5432",
		"failed to connect to admin:p@ss:w0rd@db.prod:5432: " +
			"connection refused": "db.prod:5432",
		"dial postgres://admin:p@ss:w0rd@db.prod:5432/app: " +
			"i/o timeout": "db.prod:5432",
	}
	for line, want := range cases {
		got, ok := DependencyIn(line)
		if !ok || got.Host != want {
			t.Errorf("%q: got %+v (%v), want host %q", line, got, ok, want)
		}
		call, ok := endpointIn(line)
		if !ok || call.endpoint != want {
			t.Errorf("%q: endpoint %+v (%v), want %q", line, call, ok, want)
		}
		for _, secret := range []string{"55word", "w0rd", "ss"} {
			if strings.Contains(got.Label()+call.endpoint, secret) {
				t.Errorf("%q: %q has %q", line, got.Label(), secret)
			}
		}
	}
}

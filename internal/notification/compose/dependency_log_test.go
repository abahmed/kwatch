package compose

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

func dependencyCause(mode, line string) *rootcause.CauseRecord {
	return &rootcause.CauseRecord{Rule: explain.DependencyLogRow,
		Mode: detection.Mode(mode), SharedError: line}
}

func TestDependencyWordsNameTheDependencyAndQuoteTheLog(t *testing.T) {
	cause := dependencyCause("", "org.postgresql.util.PSQLException: "+
		"Connection to db:5432 refused.")
	got := dependencyWords(cause)
	if !strings.HasPrefix(got, "it cannot connect to PostgreSQL at "+
		"db:5432 (from its log: ") {
		t.Fatalf("words = %q", got)
	}
	if causeCategory(cause) != categoryDependency {
		t.Fatalf("category = %q", causeCategory(cause))
	}
}

func TestDependencyStepsCheckTheServiceOrTheAddress(t *testing.T) {
	inCluster := dependencyCause(string(explain.ModeDependencyService),
		"dial tcp cache.data:6379: connect: connection refused")
	steps := dependencySteps(inCluster, "web", "api")
	if len(steps) != 1 || steps[0].Command !=
		"kubectl get svc,endpoints cache -n data" {
		t.Fatalf("steps = %+v", steps)
	}
	named := dependencyCause(string(explain.ModeDependencyUnreachable),
		"RedisConnectionException: It was not possible to connect")
	steps = dependencySteps(named, "web", "api")
	if len(steps) != 1 || steps[0].Command != "" || steps[0].Text !=
		"Check the Redis address in api's configuration and that it "+
			"is reachable from web" {
		t.Fatalf("steps = %+v", steps)
	}
}

// A log line that carries a URL with a user and password names the
// host after them; no part of the message shows the password or reads
// the user and password as a host and port.
func TestDependencyMessageNeverShowsUserinfo(t *testing.T) {
	for _, line := range []string{
		"failed to connect to postgres://admin:12345@db:5432/app: " +
			"connection refused",
		"could not connect to redis://default:98765@cache:6379 " +
			"connection refused",
		"Error: connect ECONNREFUSED mongodb://root:12345@mongo/admin",
	} {
		p := podCrash(incident.Notify)
		p.Cause = dependencyCause(
			string(explain.ModeDependencyUnreachable), line)
		p.Cause.Root, p.Cause.Score = p.Root, 0.6
		msg := Writer{}.Write(announce(p), writerNow)
		body, err := json.Marshal(msg)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"12345", "98765"} {
			if strings.Contains(string(body), secret) {
				t.Errorf("%q: message shows %q: %s", line, secret, body)
			}
		}
		if words := dependencyWords(p.Cause); strings.Contains(words,
			"12345") || strings.Contains(words, "98765") {
			t.Errorf("%q: words = %q", line, words)
		}
		steps := dependencySteps(p.Cause, "web", "api")
		if len(steps) != 1 || strings.Contains(steps[0].Text, "admin") ||
			strings.Contains(steps[0].Text, "default:") {
			t.Errorf("%q: steps = %+v", line, steps)
		}
	}
}

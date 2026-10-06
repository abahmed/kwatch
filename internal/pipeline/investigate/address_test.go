package investigate

import (
	"strings"
	"testing"
)

func TestAnnotateAddresses(t *testing.T) {
	known := map[string]string{
		"10.0.3.4": "db/postgres", "10.0.3.5": "db/cache",
	}
	lookup := func(ip string) (string, bool) {
		name, ok := known[ip]
		return name, ok
	}
	tests := []struct{ name, in, want string }{
		{"ip and port",
			"dial tcp 10.0.3.4:5432: connect: connection refused",
			"dial tcp 10.0.3.4:5432 (db/postgres): connect: " +
				"connection refused"},
		{"bare ip", "peer 10.0.3.4 gone", "peer 10.0.3.4 (db/postgres) gone"},
		{"unknown stays", "dial 10.9.9.9:80", "dial 10.9.9.9:80"},
		{"named once", "10.0.3.4 then 10.0.3.4:1",
			"10.0.3.4 (db/postgres) then 10.0.3.4:1"},
		{"two owners", "10.0.3.4 and 10.0.3.5",
			"10.0.3.4 (db/postgres) and 10.0.3.5 (db/cache)"},
		{"already named", "10.0.3.4:5432 (db/postgres) down",
			"10.0.3.4:5432 (db/postgres) down"},
		{"version is not an address", "v1.10.0.3.4 and 10.0.3.4.7",
			"v1.10.0.3.4 and 10.0.3.4.7"},
		{"no address", "plain line", "plain line"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := annotateAddresses(tc.in, lookup); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBoundedKeepsPrivateAddressesButRedactsCredentials(t *testing.T) {
	r := Bounded(Result{Output: []string{
		"dial tcp 10.0.3.4:5432: refused, password=hunter2"}})
	got := r.Output[0]
	if !strings.Contains(got, "10.0.3.4:5432") {
		t.Errorf("line %q should keep the private address", got)
	}
	if strings.Contains(got, "hunter2") {
		t.Errorf("line %q leaks the password", got)
	}
}

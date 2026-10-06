package format

import "testing"

func TestSignatureJoinsWhatDiffersBetweenReplicas(t *testing.T) {
	tests := []struct{ name, a, b string }{
		{"ip and port",
			"dial tcp 10.4.7.21:6379: connect: connection refused",
			"dial tcp 10.9.0.3:6379: connect: connection refused"},
		{"timestamps",
			"2026-09-29T10:00:01.123Z ERROR request failed",
			"2026-09-29 11:42:59+02:00 error request failed"},
		{"uuid",
			"job 4f1c2a9e-0b7d-4c3e-9a51-2d3c4b5a6f70 failed",
			"job 0a1b2c3d-4e5f-6a7b-8c9d-0e1f2a3b4c5d failed"},
		{"hex address", "nil pointer at 0xc000123abc",
			"nil pointer at 0xc000ffee00"},
		{"long hex id", "trace abcdef0123456789 aborted",
			"trace 0123456789abcdef aborted"},
		{"pod name", "worker api-7f9c6d8b5-x2k4p crashed",
			"worker api-5d8f7c6b4-q8z2m crashed"},
		{"client port", "read tcp 10.0.0.5:51234->10.0.0.9:5432: reset",
			"read tcp 10.0.0.6:40111->10.0.0.9:5432: reset"},
		{"ipv6", "dial tcp [fe80::1]:6379: connection refused",
			"dial tcp [2001:db8::7]:6379: connection refused"},
		{"numbers", "failed after 31 retries", "failed after 7 retries"},
		{"clock time", "10:00:01 fatal: stop", "23:59:59 fatal: stop"},
		{"spacing and case", "Disk  FULL", "disk full"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if a, b := Signature(tt.a), Signature(tt.b); a != b {
				t.Fatalf("signatures differ:\n%q\n%q", a, b)
			}
		})
	}
}

func TestSignatureKeepsDifferentErrorsApart(t *testing.T) {
	if Signature("disk full") == Signature("permission denied") {
		t.Fatal("different errors must keep different signatures")
	}
}

func TestSignatureNormalisesRedisRefusal(t *testing.T) {
	got := Signature("dial tcp 10.4.7.21:6379: connect: connection refused")
	want := "dial tcp <ip>:6379: connect: connection refused"
	if got != want {
		t.Fatalf("Signature = %q, want %q", got, want)
	}
}

func TestIsGenericSignature(t *testing.T) {
	generic := []string{"", "Error", "exit status 1", "panic:", "Killed",
		"goroutine 1 [running]:", "fatal error:", "OOMKilled"}
	for _, text := range generic {
		if !IsGenericSignature(Signature(text)) {
			t.Errorf("%q should be generic", text)
		}
	}
	specific := []string{"panic: license check failed",
		"dial tcp 10.0.0.1:6379: connect: connection refused"}
	for _, text := range specific {
		if IsGenericSignature(Signature(text)) {
			t.Errorf("%q should not be generic", text)
		}
	}
}

func TestSignatureKeepsBackendsAndStatusCodesApart(t *testing.T) {
	apart := [][2]string{
		{"dial tcp 10.4.7.21:5432: connect: connection refused",
			"dial tcp 10.4.7.21:6379: connect: connection refused"},
		{"upstream returned http 503", "upstream returned http 404"},
		{"request failed with status code: 500",
			"request failed with status code: 502"},
		{"dial tcp [::1]:5432: refused", "dial tcp [::1]:6379: refused"},
	}
	for _, pair := range apart {
		if Signature(pair[0]) == Signature(pair[1]) {
			t.Errorf("%q and %q must stay apart", pair[0], pair[1])
		}
	}
}

func TestSignatureLeavesHyphenatedWordsAlone(t *testing.T) {
	got := Signature("missing header x-forwarded-proto")
	if got != "missing header x-forwarded-proto" {
		t.Fatalf("Signature = %q", got)
	}
}

func TestSignatureJoinsNumberedNames(t *testing.T) {
	tests := []struct{ name, a, b string }{
		{"worker", "worker7 lost its lease", "worker3 lost its lease"},
		{"node", "node1 is not ready", "node2 is not ready"},
		{"kafka", "kafka1 unreachable", "kafka9 unreachable"},
		{"underscore", "app_user42 locked", "app_user7 locked"},
		{"version", "unsupported v1 api", "unsupported v2 api"},
		{"protocol", "bad http2 frame", "bad http1 frame"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if a, b := Signature(tt.a), Signature(tt.b); a != b {
				t.Fatalf("signatures differ:\n%q\n%q", a, b)
			}
		})
	}
}

func TestSignatureKeepsNamedHostPortsApart(t *testing.T) {
	apart := [][2]string{
		{"dial tcp db.example.com:5432: connection refused",
			"dial tcp cache.example.com:6379: connection refused"},
		{"dial tcp db.example.com:5432: connection refused",
			"dial tcp db.example.com:6379: connection refused"},
	}
	for _, pair := range apart {
		if Signature(pair[0]) == Signature(pair[1]) {
			t.Errorf("%q and %q must stay apart", pair[0], pair[1])
		}
	}
}

func TestSignatureNamedHostKeepsServicePortOnly(t *testing.T) {
	got := Signature("dial tcp db.example.com:5432: connection refused")
	want := "dial tcp db.example.com:5432: connection refused"
	if got != want {
		t.Fatalf("Signature = %q, want %q", got, want)
	}
	a := Signature("read tcp db.example.com:40111: reset")
	b := Signature("read tcp db.example.com:51234: reset")
	if a != b {
		t.Fatalf("ephemeral ports must join: %q vs %q", a, b)
	}
}

// A name keeps its letters: only its number goes, so replicas join
// while two different keys stay apart.
func TestSignatureKeepsTheLettersOfNumberedNames(t *testing.T) {
	got := Signature("panic: missing key DB_PASSWORD_V2")
	if got != "panic: missing key db_password_v#" {
		t.Fatalf("Signature = %q", got)
	}
	if Signature("worker7 lost its lease") != "worker# lost its lease" {
		t.Fatalf("Signature = %q", Signature("worker7 lost its lease"))
	}
	if Signature("missing key DB_PASSWORD_V2") ==
		Signature("missing key API_TOKEN_2") {
		t.Fatal("different keys must keep different signatures")
	}
	if Signature("shard db2 down") != "shard # down" {
		t.Fatalf("hex-like words become #: %q", Signature("shard db2 down"))
	}
}

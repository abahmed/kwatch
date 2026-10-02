package notification

import "testing"

// Paging systems must keep deduplicating on the same key after a state
// reset, so a stable DedupKey wins over the conversation key.
func TestAlertKeyPrefersStableDedupKey(t *testing.T) {
	cases := []struct {
		name    string
		msg     Message
		cluster string
		want    string
	}{
		{"dedup key with cluster",
			Message{Key: "inc-1", DedupKey: "a1b2"}, "prod eu",
			"kwatch-prod-eu-a1b2"},
		{"dedup key without cluster",
			Message{Key: "inc-1", DedupKey: "a1b2"}, "", "kwatch-a1b2"},
		{"falls back to conversation key",
			Message{Key: "inc-1"}, "prod", "kwatch-prod-inc-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.msg.AlertKey(tc.cluster); got != tc.want {
				t.Fatalf("AlertKey = %q, want %q", got, tc.want)
			}
		})
	}
	if got := (Message{Key: "inc-1", DedupKey: "a1b2"}).ThreadKey(); got !=
		"kwatch-inc-1" {
		t.Fatalf("ThreadKey must keep the conversation key, got %q", got)
	}
}

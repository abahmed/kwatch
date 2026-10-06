package explain

import "testing"

func TestReplicaCountUsesBothAndAll(t *testing.T) {
	want := map[int]string{
		1: "1 replica", 2: "both replicas", 3: "all 3 replicas",
	}
	for n, text := range want {
		if got := replicaCount(n); got != text {
			t.Errorf("replicaCount(%d) = %q, want %q", n, got, text)
		}
	}
}

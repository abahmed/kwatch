package inventory

import (
	"math/rand/v2"
	"strings"
	"testing"
)

func TestCompareIDsMatchesComparingTheirStrings(t *testing.T) {
	words := []string{"", "a", "a-b", "a.b", "ab", "b", "pod", "pods",
		"pod.x", "x/y", "Z", "é"}
	pick := func(r *rand.Rand) string { return words[r.IntN(len(words))] }
	r := rand.New(rand.NewPCG(1, 2))
	ids := make([]EntityID, 0, 400)
	for range 400 {
		ids = append(ids, NewEntityID(pick(r), Kind(pick(r)), pick(r),
			pick(r)))
	}
	for _, a := range ids {
		for _, b := range ids {
			want := strings.Compare(a.String(), b.String())
			if got := CompareIDs(a, b); got != want {
				t.Fatalf("CompareIDs(%q, %q) = %d, want %d",
					a.String(), b.String(), got, want)
			}
		}
	}
}

func BenchmarkCompareIDs(b *testing.B) {
	x := CoreID("pod", "shop", "api-7d9f-abcde")
	y := CoreID("pod", "shop", "api-7d9f-abcdf")
	for range b.N {
		CompareIDs(x, y)
	}
}

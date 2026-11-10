package audit

import (
	"strings"
	"testing"
)

func TestDescribe(t *testing.T) {
	cases := []struct {
		name string
		e    Entry
		want string
	}{
		{"namespace in root", Entry{Action: ActionUpdate,
			Root: "deployment/shop/api", Namespace: "shop"},
			"deployment/api in shop: alert updated"},
		{"update", Entry{Action: ActionUpdate, Root: "deployment/warehouse",
			Namespace: "staging", DecisionReason: "still open"},
			"deployment/warehouse in staging: alert updated (still open)"},
		{"create", Entry{Action: ActionCreate, Root: "pod/a",
			Namespace: "x", DecisionReason: "settled"},
			"pod/a in x: alert sent (settled)"},
		{"resolved", Entry{Action: ActionResolved, Root: "pod/a",
			Namespace: "x", DecisionReason: "healthy for 3m0s"},
			"pod/a in x: resolved (healthy for 3m0s)"},
		{"subject wins", Entry{Action: ActionUpdate, Root: "node/n1",
			Subject: "pod/a", Namespace: "x"},
			"pod/a in x: alert updated"},
		{"summary", Entry{Action: ActionSummary,
			DecisionReason: "restored incidents"},
			"summary sent (restored incidents)"},
		{"dropped", Entry{Action: ActionCreate, Root: "pod/a",
			Delivery: "dropped"}, "pod/a: left out of the digest"},
	}
	for _, c := range cases {
		if got := Describe(c.e); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDescribeIsCapped(t *testing.T) {
	e := Entry{Action: ActionCreate, Root: strings.Repeat("x", 500)}
	if got := Describe(e); len([]rune(got)) != maxMessage ||
		!strings.HasSuffix(got, "...") {
		t.Fatalf("len %d", len(got))
	}
}

func TestLevelFor(t *testing.T) {
	for tier, want := range map[string]string{
		"page": LevelError, "notify": LevelWarning,
		"digest": LevelInfo, "silent": LevelInfo, "": LevelInfo,
	} {
		if got := LevelFor(tier); got != want {
			t.Errorf("%q: %s, want %s", tier, got, want)
		}
	}
}

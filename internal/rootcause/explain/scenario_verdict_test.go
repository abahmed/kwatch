package explain_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// expectation is the part of a scenario's expect.json this engine is
// judged on.
type expectation struct {
	Name         string   `json:"name"`
	Root         string   `json:"root"`
	MustNotBlame []string `json:"mustNotBlame"`
}

// verdict is how the engine did on one scenario.
type verdict struct {
	name    string
	roots   []string
	blamed  []string
	areas   int
	rootHit bool
}

func judge(t *testing.T, e expectation) verdict {
	ex := explain.Explain(snapshotOf(loadScenario(t, e.Name)))
	out := verdict{name: e.Name, areas: len(ex.Areas)}
	forbidden := map[string]bool{}
	for _, id := range e.MustNotBlame {
		forbidden[id] = true
	}
	for _, area := range ex.Areas {
		for _, c := range area.Causes {
			root := c.Root.String()
			out.roots = append(out.roots, root)
			out.rootHit = out.rootHit || root == e.Root
			// A cause that covers only itself blames nothing else.
			if forbidden[root] && len(c.Covers) > 1 {
				out.blamed = append(out.blamed, root)
			}
		}
	}
	return out
}

func readExpectations(t *testing.T) []expectation {
	files, err := filepath.Glob(filepath.Join(scenarioDir, "*.expect.json"))
	if err != nil {
		t.Fatal(err)
	}
	var out []expectation
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var e expectation
		if err := json.Unmarshal(data, &e); err != nil {
			t.Fatal(err)
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// mustPass are the scenarios whose expected root the engine finds
// without blaming a must-not-blame entity. The list only grows.
var mustPass = map[string]bool{
	"bad-rollout": true, "cause-revised": true,
	"cold-start-preexisting": true, "configmap-change": true,
	"cordoned-node-app-crash": true, "coredns-down": true,
	"cronjob-invalid-schedule": true, "flapping-workload": true,
	"healthy-node-app-crash": true, "image-typo": true,
	"metrics-apiservice-down": true, "missing-secret": true,
	"networkpolicy-change": true, "node-lost-notready": true,
	"node-memory-pressure-eviction": true, "oom-limit-too-low": true,
	"operator-cr-stuck": true, "pvc-pending-immediate": true,
	"pvc-wffc-no-consumer": true, "quota-exhausted": true,
	"registry-auth-failure": true, "scheduler-insufficient-memory": true,
	"secret-key-removed": true, "two-independent-problems": true,
	"webhook-no-endpoints": true, "zone-failure": true,
}

// TestScenarioLogs runs the engine over every recorded scenario log
// and prints a verdict table; the scenarios in mustPass must pass.
func TestScenarioLogs(t *testing.T) {
	for _, e := range readExpectations(t) {
		v := judge(t, e)
		// A scenario without a root expects no incident at all.
		rooted := v.rootHit || (e.Root == "" && len(v.roots) == 0)
		pass := rooted && len(v.blamed) == 0
		t.Logf("%-32s pass=%-5v areas=%d want=%s got=%s blamed=%v",
			v.name, pass, v.areas, e.Root, strings.Join(v.roots, ","),
			v.blamed)
		if mustPass[e.Name] && !pass {
			t.Errorf("%s regressed", e.Name)
		}
	}
}

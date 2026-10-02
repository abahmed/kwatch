package explain_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// verdictOf renders the causes and unexplained failures of some areas
// in a canonical order, so two solves can be compared.
func verdictOf(areas []explain.Area) string {
	var lines []string
	for _, area := range areas {
		for _, c := range area.Causes {
			lines = append(lines, "cause "+c.Root.String()+" covers "+
				joinIDs(c.Covers))
		}
		for _, id := range area.Unexplained {
			lines = append(lines, "unexplained "+id.String())
		}
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func joinIDs(ids []inventory.EntityID) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, id.String())
	}
	return strings.Join(parts, ",")
}

// TestSolverMatchesFullSolveOnScenarios solves every scenario in two
// steps, half of the failures first and the rest as dirty, and expects
// the cached areas to say what one full solve says.
func TestSolverMatchesFullSolveOnScenarios(t *testing.T) {
	for _, e := range readExpectations(t) {
		t.Run(e.Name, func(t *testing.T) {
			full := snapshotOf(loadScenario(t, e.Name))
			want := verdictOf(explain.Explain(full).Areas)

			ids := make([]inventory.EntityID, 0, len(full.Findings))
			for id := range full.Findings {
				ids = append(ids, id)
			}
			sort.Slice(ids, func(i, j int) bool {
				return ids[i].String() < ids[j].String()
			})
			half := full
			half.Findings = map[inventory.EntityID][]detection.Finding{}
			for _, id := range ids[:len(ids)/2] {
				half.Findings[id] = full.Findings[id]
			}
			solver := explain.NewSolver()
			solver.Solve(half, nil)
			solver.Solve(full, ids[len(ids)/2:])
			if got := verdictOf(solver.Areas()); got != want {
				t.Fatalf("incremental:\n%s\nfull:\n%s", got, want)
			}
			if again := solver.Solve(full, nil); len(again) != 0 {
				t.Fatalf("a steady snapshot solved %d areas again",
					len(again))
			}
		})
	}
}

package dashboard

import (
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// addWeekOpenings sets each tile's Week: how many times issues opened
// in its namespace in the last trendDays days, counted as /api/trends
// counts its namespaces (countSpace), for every tile shown rather than
// the trendTop namespaces alone.
func addWeekOpenings(
	m *InsightNamespaces, items []incident.Incident, now time.Time,
	zone *time.Location,
) {
	if len(m.Tiles) == 0 {
		return
	}
	want := map[string]int{}
	for _, t := range m.Tiles {
		want[t.Namespace] = 0
	}
	w := newTrendWeek(now, zone)
	for _, p := range items {
		ns := p.Root.Namespace
		if _, ok := want[ns]; !ok || p.Tier == incident.Silent {
			continue
		}
		for _, at := range openings(p) {
			if w.day(at) >= 0 {
				want[ns]++
			}
		}
	}
	for i := range m.Tiles {
		m.Tiles[i].Week = want[m.Tiles[i].Namespace]
	}
}

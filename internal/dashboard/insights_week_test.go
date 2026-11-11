package dashboard

import (
	"fmt"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// Every namespace tile says how many times issues opened in it this
// week, not only the trendTop namespaces /api/trends lists; an issue
// that opened before the week counts none.
func TestNamespaceTilesCarryTheWeekForEveryTile(t *testing.T) {
	var items []incident.Incident
	for i := range trendTop + 2 {
		ns := fmt.Sprintf("ns-%d", i)
		items = append(items, opened("a"+ns, ns, incident.Notify,
			ago(30*time.Hour), ago(time.Hour)))
	}
	items = append(items, opened("old", "ns-0", incident.Notify,
		ago(10*24*time.Hour)))
	out := buildInsights(insightInput{items: items, now: trendNow})
	if len(out.Namespaces.Tiles) != trendTop+2 {
		t.Fatalf("tiles = %+v", out.Namespaces.Tiles)
	}
	for _, tile := range out.Namespaces.Tiles {
		if tile.Week != 2 {
			t.Errorf("%s: week = %d, want 2", tile.Namespace, tile.Week)
		}
	}
}

package dashboard

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// "Show more" asks for more events with ?limit=; the counts by kind
// cover the whole window whatever the limit.
func TestActivityHandlerTakesALimitAndCountsKinds(t *testing.T) {
	items := make([]incident.Incident, 0, 30)
	for i := range 30 {
		p := testIncident("i"+strconv.Itoa(i), "ns", -time.Minute)
		p.Occurrences = []time.Time{p.Opened}
		items = append(items, p)
	}
	s := newTestServer(items...)
	for _, path := range []string{"/api/activity?limit=0",
		"/api/activity?limit=201", "/api/activity?limit=x",
		"/api/activity?limit=5&limit=6"} {
		w := get(t, s, path)
		if w.Code != http.StatusBadRequest ||
			!strings.Contains(w.Body.String(), `"param":"limit"`) {
			t.Errorf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
	for path, n := range map[string]int{"/api/activity": maxActivity,
		"/api/activity?limit=5": 5, "/api/activity?limit=200": 30} {
		var r ActivityResponse
		w := get(t, s, path)
		if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil ||
			len(r.Events) != n || r.Total != 30 ||
			r.Kinds[activityNew] != 30 {
			t.Errorf("%s: %d events of %d, kinds %v", path,
				len(r.Events), r.Total, r.Kinds)
		}
	}
}

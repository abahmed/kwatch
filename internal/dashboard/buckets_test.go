package dashboard

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// bucketIssues are one issue of each bucket and a resolved one:
// "fresh" opened an hour ago, "old" two days ago and unchanged, "back"
// two days ago but came back an hour ago, "stuck" a stuck deletion and
// "left" a leftover group.
func bucketIssues() []incident.Incident {
	fresh := testIncident("fresh", "shop", -time.Hour)
	old := testIncident("old", "shop", -48*time.Hour)
	back := testIncident("back", "shop", -48*time.Hour)
	back.Occurrences = []time.Time{testEpoch.Add(-time.Hour)}
	stuck := testIncident("stuck", "ops", -time.Hour)
	stuck.Tier = incident.Digest
	f := detection.Finding{Entity: inventory.CoreID("namespace", "",
		"ops"), Reason: reasons.StuckDeleting}
	stuck.Members = map[detection.Key]detection.Finding{f.Key(): f}
	done := testIncident("done", "shop", -3*time.Hour)
	done.State, done.Resolved = incident.Resolved, testEpoch
	return []incident.Incident{fresh, old, back, stuck, leftoverIssue(),
		done}
}

func TestBucketsSplitTheActiveList(t *testing.T) {
	s := newTestServer(bucketIssues()...)
	for _, tc := range []struct{ query, ids string }{
		{"bucket=attention", "back fresh"},
		{"bucket=ongoing", "old"},
		{"bucket=cleanup", "left stuck"},
		{"", "back fresh left old stuck"},
		{"status=resolved&bucket=attention", ""},
	} {
		w := get(t, s, "/api/incidents?"+tc.query)
		require.Equal(t, http.StatusOK, w.Code, tc.query)
		l := decodeList(t, w.Body.String())
		ids := strings.Split(listIDs(l), ",")
		slices.Sort(ids)
		assert.Equal(t, tc.ids, strings.TrimSpace(strings.Join(ids, " ")),
			tc.query)
	}
	l := decodeList(t, get(t, s, "/api/incidents?bucket=ongoing").Body.
		String())
	assert.Equal(t, map[string]int{bucketAttention: 2, bucketOngoing: 1,
		bucketCleanup: 2}, l.Facets.Bucket)
	// The status switch still counts every active issue.
	assert.Equal(t, 5, l.Facets.State["open"])
	assert.Equal(t, 1, l.Facets.State["resolved"])
}

func TestBucketRefusesUnknownValues(t *testing.T) {
	s := newTestServer(bucketIssues()...)
	for _, q := range []string{"bucket=later",
		"bucket=ongoing&bucket=cleanup"} {
		assert.Equal(t, http.StatusBadRequest,
			get(t, s, "/api/incidents?"+q).Code, q)
	}
}

func TestUrgentLeftoverNeedsAttention(t *testing.T) {
	p := leftoverIssue()
	p.Tier = incident.Page
	item := toListItem(p)
	assert.Equal(t, bucketAttention, bucketOf(p, item, testEpoch))
}

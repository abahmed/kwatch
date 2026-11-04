package dashboard

import (
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
)

// The severity facets add up to the list's total: a quiet issue that
// is still active, which the list hides without a severity filter, is
// not counted either.
func TestListSeverityFacetsAddUpToTheTotal(t *testing.T) {
	quiet := testIncident("q", "shop", -time.Minute)
	quiet.Tier = incident.Silent
	quietDone := testIncident("d", "shop", -time.Hour)
	quietDone.Tier = incident.Silent
	quietDone.State, quietDone.Resolved = incident.Resolved, testEpoch
	loud := testIncident("l", "shop", -time.Minute)
	items := []incident.Incident{quiet, quietDone, loud}
	req, err := parseListRequest(url.Values{"status": {"all"}}, testEpoch)
	require.NoError(t, err)

	got := buildList(req, items, testEpoch, func(s string) string {
		return s
	})

	sum := 0
	for _, n := range got.Facets.Severity {
		sum += n
	}
	assert.Equal(t, got.Total, sum)
	assert.Equal(t, 2, got.Total)
}

package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A flapper that starts paging widens the route its alert was opened
// with, so a pager that asks for critical alerts hears the resolve. The
// announcement of a held page is routed at the tier it was held at.
func TestFlapperPageWidensTheAnnouncedRoute(t *testing.T) {
	r, _ := pagedRig(t)
	resolved := resolvePage(t, r, 5*time.Minute)
	_, raised := failAgain(t, r, resolved, 10*time.Minute)
	resolved = resolvePage(t, r, raised+10*time.Minute)
	r.m.incidents[r.only().ID].AnnouncedRoute.Severity = "warning"

	second, _ := failAgain(t, r, resolved, 10*time.Minute)

	require.Equal(t, Page, second.Incident.Tier, "setup: it pages")
	assert.Equal(t, "critical", second.Incident.AnnouncedRoute.Severity)
	assert.Equal(t, "critical", r.only().AnnouncedRoute.Severity,
		"kept, so the resolve is routed with it")
}

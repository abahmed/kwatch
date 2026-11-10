package pagerduty

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// The Events API refuses an event over 512 KB for good, so a huge output
// is cut instead of losing the page.
func TestPagerDutyCutsOverlongOutput(t *testing.T) {
	c, rec := newTestPagerDuty(t)
	m := providertest.Announce()
	m.Output = []string{strings.Repeat("<", 400000)}
	m.Note = strings.Repeat("n", 400000)
	require.NoError(t, c.SendIncident(context.Background(), m))
	assert.Less(t, len(rec.Last(t).Body), 512*1024)
	assert.Contains(t, string(rec.Last(t).Body), "output")
}

package newrelic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func newRelicIn(t *testing.T, region string) *NewRelic {
	t.Helper()
	rec := providertest.NewRecorder(t)
	config := map[string]interface{}{"apiKey": "k", "accountId": "42"}
	if region != "" {
		config["region"] = region
	}
	c := NewNewRelic(config, "dev", rec.Dependencies())
	require.NotNil(t, c)
	return c
}

func TestNewRelicRegionPicksTheCollector(t *testing.T) {
	assert.Equal(t, "https://insights-collector.newrelic.com/v1/"+
		"accounts/42/events", newRelicIn(t, "").url)
	assert.Equal(t, newRelicIn(t, "").url, newRelicIn(t, "us").url)
	assert.Equal(t, "https://insights-collector.eu01.nr-data.net/v1/"+
		"accounts/42/events", newRelicIn(t, "EU").url)
	assert.Equal(t, newRelicIn(t, "").url, newRelicIn(t, "mars").url)
}

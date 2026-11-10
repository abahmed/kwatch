package datadog

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// An aggregation key longer than an event accepts is capped.
func TestDatadogAggregationKeyIsCapped(t *testing.T) {
	c, rec := newTestDatadog(t, nil)
	msg := providertest.Update()
	msg.Key = strings.Repeat("k", 300)

	require.NoError(t, c.SendIncident(context.Background(), msg))

	key := rec.Last(t).JSON(t)["aggregation_key"].(string)
	assert.LessOrEqual(t, len(key), 100)
	assert.Equal(t, msg.AlertKeyMax("dev", 100), key)
}

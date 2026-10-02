package zapier

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestZapierSendIncidentLifecycle(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewZapier(map[string]interface{}{
		"url": rec.URL(), "title": "custom",
	}, "dev", rec.Dependencies())
	require.NotNil(t, c)

	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			m := tc.Message
			require.NoError(t, c.SendIncident(context.Background(), m))
			got := rec.Last(t).JSON(t)
			assert.Equal(t, "dev", got["cluster"])
			assert.Equal(t, providertest.Key, got["key"])
			assert.Equal(t, "kwatch-dev-"+providertest.Key, got["alertKey"])
			assert.EqualValues(t, m.Revision, got["revision"])
			assert.Equal(t, m.Status.String(), got["status"])
			assert.Equal(t, m.Resolved(), got["resolved"])
			assert.Equal(t, m.Marker, got["marker"])
			assert.Equal(t, m.Short, got["short"])
			assert.Equal(t, m.Note, got["note"])
			assert.Equal(t, m.Title, got["title"])
			assert.Contains(t, got, "route")
			providertest.AssertOneLeadingEmoji(t, got["note"].(string))
		})
	}
}

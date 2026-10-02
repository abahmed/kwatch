package flock

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestFlockSendIncidentRendersNote(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewFlock(map[string]interface{}{"webhook": rec.URL()}, "dev",
		rec.Dependencies())
	require.NotNil(t, c)
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			m := tc.Message
			require.NoError(t, c.SendIncident(context.Background(), m))
			text := rec.Last(t).JSON(t)["text"].(string)
			assert.True(t, strings.HasPrefix(text, m.Note))
			providertest.AssertOneLeadingEmoji(t, text)
		})
	}
}

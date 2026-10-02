package signal

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestSignalSendIncidentRendersNote(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewSignal(map[string]interface{}{
		"number": "+12025550199", "to": "+12025550100", "url": rec.URL(),
	}, "dev", rec.Dependencies())
	require.NotNil(t, c)
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			m := tc.Message
			require.NoError(t, c.SendIncident(context.Background(), m))
			req := rec.Last(t)
			assert.Equal(t, "/v2/send", req.Path)
			body := req.JSON(t)
			assert.Equal(t, []any{"+12025550100"}, body["recipients"])
			text := body["message"].(string)
			assert.True(t, strings.HasPrefix(text, m.Note))
			providertest.AssertOneLeadingEmoji(t, text)
		})
	}
}

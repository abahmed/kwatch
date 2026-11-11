package teams

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/ratelimit"
)

func connectorTeams(
	t *testing.T, body string,
) (*Teams, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}
	c := NewTeams(map[string]interface{}{"webhook": rec.URL()},
		"dev", rec.Dependencies())
	require.NotNil(t, c)
	return c, rec
}

const connectorFailed = "Webhook message delivery failed with error: " +
	"Microsoft Teams endpoint returned HTTP error "

// A legacy connector answers 200 when Teams throttled the message. That
// is a rate limit, not a delivery.
func TestTeamsConnector200WithA429InsideIsARateLimit(t *testing.T) {
	c, _ := connectorTeams(t, connectorFailed+"429 with ContextId x.")

	err := c.SendIncident(context.Background(), providertest.Announce())

	var limited *ratelimit.Error
	require.True(t, errors.As(err, &limited), "got %v", err)
	assert.Equal(t, http.StatusTooManyRequests, limited.StatusCode)
}

// A refused message (too large) is permanent; a server-side failure is
// retried; the success body "1" is a delivery.
func TestTeamsConnector200WithAnErrorInsideIsAFailure(t *testing.T) {
	c, _ := connectorTeams(t, connectorFailed+"413 with ContextId x.")
	err := c.SendIncident(context.Background(), providertest.Announce())
	require.Error(t, err)
	assert.True(t, transport.IsPermanent(err))

	c, _ = connectorTeams(t, connectorFailed+"502 with ContextId x.")
	err = c.SendIncident(context.Background(), providertest.Announce())
	require.Error(t, err)
	assert.False(t, transport.IsPermanent(err))

	c, _ = connectorTeams(t, "1")
	require.NoError(t,
		c.SendIncident(context.Background(), providertest.Announce()))
}

package incidentio

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

var testDeps = transport.Dependencies{
	HTTPClient: http.DefaultClient,
}

func testAppConfig() string {
	return "dev"
}

func TestEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewIncidentio(map[string]interface{}{}, testAppConfig(), testDeps)
	assert.Nil(c)
}

func TestIncidentio(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"url": "https://webhooks.incident.io/abc",
	}
	c := NewIncidentio(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	assert.Equal(c.Name(), "Incident.io")
}

func TestIncidentioSendMessageSkipsNotice(t *testing.T) {
	assert := assert.New(t)

	var gotAuth string
	var gotBody string
	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			w.WriteHeader(http.StatusAccepted)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"url":    "https://webhooks.incident.io/abc",
		"apiKey": "test",
	}
	c := NewIncidentio(configMap, testAppConfig(), testDeps)
	c.url = s.URL

	// A plain notice must not open an alert on a paging provider.
	assert.Nil(c.SendMessage(context.Background(), "hello"))
	assert.Empty(gotAuth)
	assert.Empty(gotBody)
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"url": "https://webhooks.incident.io/abc",
	}
	c := NewIncidentio(configMap, testAppConfig(), testDeps)
	c.url = s.URL

	assert.NotNil(c.SendIncident(context.Background(), providertest.Announce()))
}

func TestInvalidHttpRequest(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"url": "https://webhooks.incident.io/abc",
	}
	c := NewIncidentio(configMap, testAppConfig(), testDeps)
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendIncident(context.Background(), providertest.Announce()))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendIncident(context.Background(), providertest.Announce()))
}

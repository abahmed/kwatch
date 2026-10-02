package squadcast

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

	c := NewSquadcast(map[string]interface{}{}, testAppConfig(), testDeps)
	assert.Nil(c)
}

func TestSquadcast(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"serviceKey": "test",
	}
	c := NewSquadcast(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	assert.Equal(c.Name(), "Squadcast")
	assert.Equal(c.url, "https://api.squadcast.com/v2/incidents/api/test")
}

func TestSquadcastSendMessageSkipsNotice(t *testing.T) {
	assert := assert.New(t)

	var gotBody string
	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			w.WriteHeader(http.StatusOK)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"serviceKey": "test",
	}
	c := NewSquadcast(configMap, testAppConfig(), testDeps)
	c.url = s.URL

	// A plain notice must not open an alert on a paging provider.
	assert.Nil(c.SendMessage(context.Background(), "hello"))
	assert.Empty(gotBody)
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"serviceKey": "test",
	}
	c := NewSquadcast(configMap, testAppConfig(), testDeps)
	c.url = s.URL

	assert.NotNil(c.SendIncident(context.Background(), providertest.Announce()))
}

func TestInvalidHttpRequest(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"serviceKey": "test",
	}
	c := NewSquadcast(configMap, testAppConfig(), testDeps)
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendIncident(context.Background(), providertest.Announce()))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendIncident(context.Background(), providertest.Announce()))
}

package goalert

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

	c := NewGoalert(map[string]interface{}{}, testAppConfig(), testDeps)
	assert.Nil(c)
}

func TestGoalert(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"token":     "test",
		"serviceId": "SVC123",
		"url":       "https://goalert.example.test",
	}
	c := NewGoalert(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	assert.Equal(c.Name(), "GoAlert")
	assert.Equal(c.url, "https://goalert.example.test/api/v2/generic/incoming")
}

func TestGoalertCustomURL(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"url":       "https://goalert.internal.test",
		"token":     "test",
		"serviceId": "SVC123",
	}
	c := NewGoalert(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	assert.Equal(c.url, "https://goalert.internal.test/api/v2/generic/incoming")
}

func TestGoalertInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewGoalert(
		map[string]interface{}{
			"serviceId": "S",
		},
		testAppConfig(),
		testDeps,
	)
	assert.Nil(c)

	c = NewGoalert(map[string]interface{}{"token": "t"}, testAppConfig(), testDeps)
	assert.Nil(c)
}

func TestGoalertSendMessageSkipsNotice(t *testing.T) {
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
		"token":     "test",
		"serviceId": "SVC123",
		"url":       "https://goalert.example.test",
	}
	c := NewGoalert(configMap, testAppConfig(), testDeps)
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
			w.WriteHeader(http.StatusForbidden)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"token":     "test",
		"serviceId": "SVC123",
		"url":       "https://goalert.example.test",
	}
	c := NewGoalert(configMap, testAppConfig(), testDeps)
	c.url = s.URL

	assert.NotNil(c.SendIncident(context.Background(), providertest.Announce()))
}

func TestInvalidHttpRequest(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"token":     "test",
		"serviceId": "SVC123",
		"url":       "https://goalert.example.test",
	}
	c := NewGoalert(configMap, testAppConfig(), testDeps)
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendIncident(context.Background(), providertest.Announce()))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendIncident(context.Background(), providertest.Announce()))
}

package teamsworkflow

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

var testDeps = transport.Dependencies{
	HTTPClient: http.DefaultClient,
}

func testAppConfig() string {
	return "dev"
}

func TestEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewTeamsWorkflow(map[string]interface{}{}, testAppConfig(), testDeps)
	assert.Nil(c)
}

func TestTeamsWorkflow(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"webhook": "https://prod-00.westeurope.logic.azure.com/triggers/manual/run/abc",
	}
	c := NewTeamsWorkflow(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	assert.Equal(c.Name(), "Teams Workflow")
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	var gotBody string
	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			w.WriteHeader(http.StatusAccepted)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"webhook": "https://prod-00.westeurope.logic.azure.com/triggers/manual/run/abc",
	}
	c := NewTeamsWorkflow(configMap, testAppConfig(), testDeps)
	c.webhook = s.URL

	assert.Nil(c.SendMessage(context.Background(), "test"))
	assert.Contains(gotBody, `"type":"message"`)
	assert.Contains(gotBody, `"contentType":"application/vnd.microsoft.card.adaptive"`)
	assert.Contains(gotBody, `"text":"test"`)
	assert.Contains(gotBody, `"type":"AdaptiveCard"`)
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"webhook": "https://prod-00.westeurope.logic.azure.com/triggers/manual/run/abc",
	}
	c := NewTeamsWorkflow(configMap, testAppConfig(), testDeps)
	c.webhook = s.URL

	assert.NotNil(c.SendMessage(context.Background(), "test"))
}

func TestSendEvent(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			assert.Contains(string(body), "OOMKILLED")
			w.WriteHeader(http.StatusAccepted)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"webhook": "https://prod-00.westeurope.logic.azure.com/triggers/manual/run/abc",
	}
	c := NewTeamsWorkflow(configMap, testAppConfig(), testDeps)
	c.webhook = s.URL

	ev := event.Event{
		PodName:   "test-pod",
		Namespace: "default",
		Reason:    "OOMKILLED",
	}
	assert.Nil(c.SendEvent(context.Background(), &ev))
}

func TestInvalidHttpRequest(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"webhook": "https://prod-00.westeurope.logic.azure.com/triggers/manual/run/abc",
	}
	c := NewTeamsWorkflow(configMap, testAppConfig(), testDeps)
	c.webhook = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage(context.Background(), "test"))

	c.webhook = "http://localhost:132323/%s"
	assert.NotNil(c.SendMessage(context.Background(), "test"))
}

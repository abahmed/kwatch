package teams

import (
	"context"
	"encoding/json"
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

	c := NewTeams(map[string]interface{}{}, testAppConfig(), testDeps)
	assert.Nil(c)
}

func TestTelegram(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"webhook": "http://example.com",
	}
	c := NewTeams(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)

	assert.Equal(c.Name(), "Microsoft Teams")
}

func TestNewTeams(t *testing.T) {
	configMap := map[string]interface{}{
		"webhook": "http://example.com",
		"title":   "Test Title",
		"text":    "Test Text",
	}
	appCfg := testAppConfig()
	teams := NewTeams(configMap, appCfg, testDeps)
	assert.NotNil(t, teams)
	assert.Equal(t, "http://example.com", teams.webhook)
	assert.Equal(t, "Test Title", teams.title)
	assert.Equal(t, "Test Text", teams.text)
}

func TestSendEvent(t *testing.T) {
	configMap := map[string]interface{}{
		"webhook": "http://example.com",
	}
	appCfg := testAppConfig()
	teams := NewTeams(configMap, appCfg, testDeps)

	e := &event.Event{
		PodName:   "test-pod",
		Namespace: "test-namespace",
		Reason:    "test-reason",
		Logs:      "test-logs",
		Events:    "test-events",
	}

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
	defer server.Close()

	teams.webhook = server.URL
	err := teams.SendEvent(context.Background(), e)
	assert.NoError(t, err)
}

func TestSendMessage(t *testing.T) {
	configMap := map[string]interface{}{
		"webhook": "http://localhost",
	}
	appCfg := testAppConfig()
	teams := NewTeams(configMap, appCfg, testDeps)

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
	defer server.Close()

	teams.webhook = server.URL
	err := teams.SendMessage(context.Background(), "test message")
	assert.NoError(t, err)
}

func TestSendMessageErrorSchemaMismatch(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`TriggerInputSchemaMismatch`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"webhook": s.URL,
	}
	appCfg := testAppConfig()
	c := NewTeams(configMap, appCfg, testDeps)
	assert.NotNil(c)

	assert.NotNil(c.SendMessage(context.Background(), "test"))
}

func TestSendMessageErrorBadRequest(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"webhook": s.URL,
	}
	appCfg := testAppConfig()
	c := NewTeams(configMap, appCfg, testDeps)
	assert.NotNil(c)

	assert.NotNil(c.SendMessage(context.Background(), "test"))
}

func TestSendMessageErrorAccepted(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"webhook": s.URL,
	}
	appCfg := testAppConfig()
	c := NewTeams(configMap, appCfg, testDeps)
	assert.NotNil(c)

	assert.Nil(
		c.SendMessage(context.Background(), "test"),
		"202 Accepted is now success",
	)
}

func TestSendMessageErrorServer(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"webhook": s.URL,
	}
	appCfg := testAppConfig()
	c := NewTeams(configMap, appCfg, testDeps)
	assert.NotNil(c)

	assert.NotNil(c.SendMessage(context.Background(), "test"))
}

func TestSendAPI(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
	defer server.Close()

	configMap := map[string]interface{}{
		"webhook": server.URL,
	}
	appCfg := testAppConfig()
	teams := NewTeams(configMap, appCfg, testDeps)

	payload :=
		[]byte(`{"title":"Test Title","text":"Test Text","attachments":[]}`)
	err := teams.sendAPI(context.Background(), payload)
	assert.NoError(t, err)
}

func TestInvaildHttpRequest(t *testing.T) {
	assert := assert.New(t)

	appCfg := testAppConfig()

	configMap := map[string]interface{}{
		"webhook": "h ttp://localhost/%s",
	}

	c := NewTeams(configMap, appCfg, testDeps)
	assert.NotNil(c)
	assert.NotNil(c.SendMessage(context.Background(), "test"))

	configMap = map[string]interface{}{
		"webhook": "http://localhost:132323",
	}

	c = NewTeams(configMap, appCfg, testDeps)
	assert.NotNil(c)
	assert.NotNil(c.SendMessage(context.Background(), "test"))
}

func TestBuildRequestBodyTeams(t *testing.T) {
	configMap := map[string]interface{}{
		"webhook": "http://example.com",
		"title":   "Test Title",
		"text":    "Test Text",
	}
	teams := NewTeams(configMap, "", testDeps)

	e := &event.Event{
		PodName:       "test-pod",
		Namespace:     "test-namespace",
		Reason:        "test-reason",
		Logs:          "test-logs",
		Events:        "test-events",
		IncludeEvents: true,
		IncludeLogs:   true,
	}

	payload, err := teams.buildRequestBodyTeams(e)
	assert.NoError(t, err)
	var result teamsFlowPayload
	err = json.Unmarshal(payload, &result)
	assert.NoError(t, err)
	assert.Equal(t, "Test Title", result.Title)
	assert.Contains(t, result.Text, "test-pod")
	assert.Contains(t, result.Text, "test-namespace")
	assert.Contains(t, result.Text, "test-reason")
	assert.Contains(t, result.Text, "test-logs")
	assert.Contains(t, result.Text, "test-events")
}

func TestBuildRequestBodyMessage(t *testing.T) {
	configMap := map[string]interface{}{
		"webhook": "http://example.com",
	}
	teams := NewTeams(configMap, "", testDeps)

	payload, err := teams.buildRequestBodyMessage("test message")
	assert.NoError(t, err)
	var result teamsFlowPayload
	err = json.Unmarshal(payload, &result)
	assert.NoError(t, err)
	assert.Equal(t, "New Alert", result.Title)
	assert.Equal(t, "test message", result.Text)
	assert.Empty(t, result.Attachment)
}

func TestNewTeamsIgnoresLegacyRetrySettings(t *testing.T) {
	configMap := map[string]interface{}{
		"webhook":    "http://example.com",
		"maxRetries": 5,
		"retryDelay": 10,
	}
	appCfg := testAppConfig()
	teams := NewTeams(configMap, appCfg, testDeps)
	assert.NotNil(t, teams)
}

func TestBuildRequestBodyTeamsGolden(t *testing.T) {
	configMap := map[string]interface{}{
		"webhook": "http://example.com",
	}
	clusterName := "production"
	teams := NewTeams(configMap, clusterName, testDeps)

	e := &event.Event{
		PodName:   "my-pod",
		Namespace: "my-namespace",
		Reason:    "OOMKilled",
	}

	b, err := teams.buildRequestBodyTeams(e)
	assert.NoError(t, err)
	payload := string(b)
	assert.Contains(t, payload, "my-pod")
	assert.Contains(t, payload, "my-namespace")
	assert.Contains(t, payload, "OOMKilled")
}

func TestBuildRequestBodyTeamsDefaultTitle(t *testing.T) {
	configMap := map[string]interface{}{
		"webhook": "http://example.com",
	}
	clusterName := ""
	teams := NewTeams(configMap, clusterName, testDeps)

	e := &event.Event{
		PodName:       "test-pod",
		Namespace:     "test-namespace",
		Reason:        "test-reason",
		Logs:          "test-logs",
		Events:        "test-events",
		NodeName:      "test-node",
		IncludeEvents: true,
		IncludeLogs:   true,
	}

	payload, err := teams.buildRequestBodyTeams(e)
	assert.NoError(t, err)
	var result teamsFlowPayload
	err = json.Unmarshal(payload, &result)
	assert.NoError(t, err)
	assert.Contains(t, result.Title, "Kwatch")
}

func TestSendEventWithCustomTitle(t *testing.T) {
	configMap := map[string]interface{}{
		"webhook": "http://example.com",
		"title":   "Custom Title",
		"text":    "Custom Text",
	}
	appCfg := testAppConfig()
	teams := NewTeams(configMap, appCfg, testDeps)

	e := &event.Event{
		PodName:       "test-pod",
		Namespace:     "test-namespace",
		Reason:        "test-reason",
		Logs:          "test-logs",
		Events:        "test-events",
		IncludeEvents: true,
		IncludeLogs:   true,
	}

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
	defer server.Close()

	teams.webhook = server.URL
	err := teams.SendEvent(context.Background(), e)
	assert.NoError(t, err)
}

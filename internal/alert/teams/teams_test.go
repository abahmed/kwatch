package teams

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

var testDeps = transport.Dependencies{
	HTTPClient: http.DefaultClient,
	Clock:      clock.RealClock{},
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
		"webhook": "https://example.test/hook",
	}

	c := NewTeams(configMap, appCfg, testDeps)
	assert.NotNil(c)
	c.webhook = "h ttp://localhost/%s"
	assert.NotNil(c.SendMessage(context.Background(), "test"))

	configMap = map[string]interface{}{
		"webhook": "http://localhost:132323",
	}

	c = NewTeams(configMap, appCfg, testDeps)
	assert.NotNil(c)
	assert.NotNil(c.SendMessage(context.Background(), "test"))
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
	assert.Len(t, result.Attachment, 1)
	assert.Equal(t, "application/vnd.microsoft.card.adaptive",
		result.Attachment[0]["contentType"])
	content, ok := result.Attachment[0]["content"].(map[string]interface{})
	assert.True(t, ok, "content should be a map")
	body, ok := content["body"].([]interface{})
	assert.True(t, ok, "body should be a slice")
	assert.Len(t, body, 2)
	titleBlock := body[0].(map[string]interface{})
	assert.Equal(t, "TextBlock", titleBlock["type"])
	assert.Equal(t, "New Alert", titleBlock["text"])
	assert.Equal(t, "Bolder", titleBlock["weight"])
	msgBlock := body[1].(map[string]interface{})
	assert.Equal(t, "TextBlock", msgBlock["type"])
	assert.Equal(t, "test message", msgBlock["text"])
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

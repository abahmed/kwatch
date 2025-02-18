package teams

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/event"
	"github.com/stretchr/testify/assert"
)

func TestEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewTeams(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestTelegram(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"webhook": "http://example.com",
	}
	c := NewTeams(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)

	assert.Equal(c.Name(), "Microsoft Teams")
}

func TestNewTeams(t *testing.T) {
	configMap := map[string]interface{}{
		"webhook": "http://example.com",
		"title":   "Test Title",
		"text":    "Test Text",
	}
	appCfg := &config.App{ClusterName: "dev"}
	teams := NewTeams(configMap, appCfg)
	assert.NotNil(t, teams)
	assert.Equal(t, "http://example.com", teams.webhook)
	assert.Equal(t, "Test Title", teams.title)
	assert.Equal(t, "Test Text", teams.text)
}

func TestSendEvent(t *testing.T) {
	configMap := map[string]interface{}{
		"webhook": "http://example.com",
	}
	appCfg := &config.App{ClusterName: "dev"}
	teams := NewTeams(configMap, appCfg)

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
	err := teams.SendEvent(e)
	assert.NoError(t, err)
}

func TestSendMessage(t *testing.T) {
	configMap := map[string]interface{}{
		"webhook": "http://localhost",
	}
	appCfg := &config.App{ClusterName: "dev"}
	teams := NewTeams(configMap, appCfg)

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
	defer server.Close()

	teams.webhook = server.URL
	err := teams.SendMessage("test message")
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
	appCfg := &config.App{ClusterName: "dev"}
	c := NewTeams(configMap, appCfg)
	assert.NotNil(c)

	assert.NotNil(c.SendMessage("test"))
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
	appCfg := &config.App{ClusterName: "dev"}
	c := NewTeams(configMap, appCfg)
	assert.NotNil(c)

	assert.NotNil(c.SendMessage("test"))
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
	appCfg := &config.App{ClusterName: "dev"}
	c := NewTeams(configMap, appCfg)
	assert.NotNil(c)

	assert.NotNil(c.SendMessage("test"))
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
	appCfg := &config.App{ClusterName: "dev"}
	c := NewTeams(configMap, appCfg)
	assert.NotNil(c)

	assert.NotNil(c.SendMessage("test"))
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
	appCfg := &config.App{ClusterName: "dev"}
	teams := NewTeams(configMap, appCfg)

	payload :=
		[]byte(`{"title":"Test Title","text":"Test Text","attachment":[]}`)
	err := teams.sendAPI(payload)
	assert.NoError(t, err)
}

func TestInvaildHttpRequest(t *testing.T) {
	assert := assert.New(t)

	appCfg := &config.App{ClusterName: "dev"}

	configMap := map[string]interface{}{
		"webhook": "h ttp://localhost/%s",
	}

	c := NewTeams(configMap, appCfg)
	assert.NotNil(c)
	assert.NotNil(c.SendMessage("test"))

	configMap = map[string]interface{}{
		"webhook": "http://localhost:132323",
	}

	c = NewTeams(configMap, appCfg)
	assert.NotNil(c)
	assert.NotNil(c.SendMessage("test"))
}

func TestBuildRequestBodyTeams(t *testing.T) {
	configMap := map[string]interface{}{
		"webhook": "http://example.com",
		"title":   "Test Title",
		"text":    "Test Text",
	}
	appCfg := &config.App{}
	teams := NewTeams(configMap, appCfg)

	e := &event.Event{
		PodName:   "test-pod",
		Namespace: "test-namespace",
		Reason:    "test-reason",
		Logs:      "test-logs",
		Events:    "test-events",
	}

	payload := teams.buildRequestBodyTeams(e)
	var result teamsFlowPayload
	err := json.Unmarshal(payload, &result)
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
	appCfg := &config.App{}
	teams := NewTeams(configMap, appCfg)

	payload := teams.buildRequestBodyMessage("test message")
	var result teamsFlowPayload
	err := json.Unmarshal(payload, &result)
	assert.NoError(t, err)
	assert.Equal(t, "New Alert", result.Title)
	assert.Equal(t, "test message", result.Text)
	assert.Empty(t, result.Attachment)
}

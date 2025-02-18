package teams

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/abahmed/kwatch/event"
    "github.com/stretchr/testify/assert"
	"github.com/stretchr/kwatch/config"
)

func TestNewTeams(t *testing.T) {
    config := map[string]interface{}{
        "flowURL": "http://example.com",
        "title":   "Test Title",
        "text":    "Test Text",
    }
    appCfg := &config.App{}
    teams := NewTeams(config, appCfg)
    assert.NotNil(t, teams)
    assert.Equal(t, "http://example.com", teams.flowURL)
    assert.Equal(t, "Test Title", teams.title)
    assert.Equal(t, "Test Text", teams.text)
}

func TestSendEvent(t *testing.T) {
    config := map[string]interface{}{
        "flowURL": "http://example.com",
    }
    appCfg := &config.App{}
    teams := NewTeams(config, appCfg)

    e := &event.Event{
        PodName:   "test-pod",
        Namespace: "test-namespace",
        Reason:    "test-reason",
        Logs:      "test-logs",
        Events:    "test-events",
    }

    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    }))
    defer server.Close()

    teams.flowURL = server.URL
    err := teams.SendEvent(e)
    assert.NoError(t, err)
}

func TestSendMessage(t *testing.T) {
    config := map[string]interface{}{
        "flowURL": "http://example.com",
    }
    appCfg := &config.App{}
    teams := NewTeams(config, appCfg)

    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    }))
    defer server.Close()

    teams.flowURL = server.URL
    err := teams.SendMessage("test message")
    assert.NoError(t, err)
}

func TestSendAPI(t *testing.T) {
    config := map[string]interface{}{
        "flowURL": "http://example.com",
    }
    appCfg := &config.App{}
    teams := NewTeams(config, appCfg)

    payload := []byte(`{"title":"Test Title","text":"Test Text","attachment":[]}`)

    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    }))
    defer server.Close()

    teams.flowURL = server.URL
    err := teams.sendAPI(payload)
    assert.NoError(t, err)
}

func TestBuildRequestBodyTeams(t *testing.T) {
    config := map[string]interface{}{
        "flowURL": "http://example.com",
        "title":   "Test Title",
        "text":    "Test Text",
    }
    appCfg := &config.App{}
    teams := NewTeams(config, appCfg)

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
    config := map[string]interface{}{
        "flowURL": "http://example.com",
    }
    appCfg := &config.App{}
    teams := NewTeams(config, appCfg)

    payload := teams.buildRequestBodyMessage("test message")
    var result teamsFlowPayload
    err := json.Unmarshal(payload, &result)
    assert.NoError(t, err)
    assert.Equal(t, "New Alert", result.Title)
    assert.Equal(t, "test message", result.Text)
    assert.Empty(t, result.Attachment)
}
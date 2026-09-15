package feishu

import (
	"context"
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
	assertions := assert.New(t)

	c := NewFeiShu(map[string]interface{}{}, testAppConfig(), testDeps)
	assertions.Nil(c)
}

func TestRocketChat(t *testing.T) {
	assertions := assert.New(t)

	configMap := map[string]interface{}{
		"webhook": "testtest",
	}
	c := NewFeiShu(configMap, testAppConfig(), testDeps)
	assertions.NotNil(c)

	assertions.Equal(c.Name(), "Fei Shu")
}

func TestBuildRequestBodyFeiShu(t *testing.T) {
	assertions := assert.New(t)
	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"isOk": true}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"webhook": s.URL,
	}
	c := NewFeiShu(configMap, testAppConfig(), testDeps)
	assertions.NotNil(c)
	ev := event.Event{
		NodeName:      "test-node",
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		Reason:        "OOMKILLED",
		Logs:          "test\ntestlogs",
		Events:        "test",
		IncludeEvents: true,
		IncludeLogs:   true,
	}
	formattedMsg := ev.FormatMarkdown(c.clusterName, "", "")

	expectMessage :=
		"{\"msg_type\":\"interactive\",\"card\":{\"config\":" +
			"{\"wide_screen_mode\":true},\"header\":{\"title\":" +
			"{\"tag\":\"plain_text\",\"content\":\"\"},\"template\":\"blue\"}," +
			"\"elements\":[{\"tag\":\"markdown\",\"content\":\"Alert: " +
			"OOMKILLED in test-pod\\n**Cluster:** dev\\n**Pod:** test-pod\\n" +
			"**Container:** test-container\\n**Namespace:** default\\n" +
			"**Node:** test-node\\n**Reason:** OOMKILLED\\n**Events:**\\n```\\n" +
			"test\\n```\\n**Logs:**\\n```\\ntest\\ntestlogs\\n```\"}]}}"

	body, err := c.buildRequestBodyFeiShu(formattedMsg)
	assertions.Nil(err)
	assertions.Equal(expectMessage, body)
}

func TestSendMessage(t *testing.T) {
	assertions := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"isOk": true}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"webhook": s.URL,
	}
	c := NewFeiShu(configMap, testAppConfig(), testDeps)
	assertions.NotNil(c)

	assertions.Nil(c.SendMessage(context.Background(), "test"))
}

func TestSendMessageError(t *testing.T) {
	assertions := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"webhook": s.URL,
	}
	c := NewFeiShu(configMap, testAppConfig(), testDeps)
	assertions.NotNil(c)

	assertions.NotNil(c.SendMessage(context.Background(), "test"))
}

func TestSendEvent(t *testing.T) {
	assertions := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"isOk": true}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"webhook": s.URL,
	}
	c := NewFeiShu(configMap, testAppConfig(), testDeps)
	assertions.NotNil(c)

	ev := event.Event{
		NodeName:      "test-node",
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		Reason:        "OOMKILLED",
		Logs:          "test\ntestlogs",
		Events: "event1-event2-event3-event1-event2-event3-event1-event2-" +
			"event3\nevent5\nevent6-event8-event11-event12",
	}
	assertions.Nil(c.SendEvent(context.Background(), &ev))
}

func TestInvalidHttpRequest(t *testing.T) {
	assertions := assert.New(t)

	configMap := map[string]interface{}{
		"webhook": "h ttp://localhost",
	}
	c := NewFeiShu(configMap, testAppConfig(), testDeps)
	assertions.NotNil(c)

	assertions.NotNil(c.SendMessage(context.Background(), "test"))

	configMap = map[string]interface{}{
		"webhook": "http://localhost:132323",
	}
	c = NewFeiShu(configMap, testAppConfig(), testDeps)
	assertions.NotNil(c)

	assertions.NotNil(c.SendMessage(context.Background(), "test"))
}

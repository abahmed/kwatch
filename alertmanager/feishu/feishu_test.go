package feishu

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/event"
	"github.com/stretchr/testify/assert"
)

func TestEmptyConfig(t *testing.T) {
	assertions := assert.New(t)

	c := NewFeiShu(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assertions.Nil(c)
}

func TestRocketChat(t *testing.T) {
	assertions := assert.New(t)

	configMap := map[string]interface{}{
		"webhook": "testtest",
	}
	c := NewFeiShu(configMap, &config.App{ClusterName: "dev"})
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
	c := NewFeiShu(configMap, &config.App{ClusterName: "dev"})
	assertions.NotNil(c)
	ev := event.Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		Reason:        "OOMKILLED",
		Logs:          "test\ntestlogs",
		Events:        "test",
	}
	formattedMsg := ev.FormatMarkdown(c.appCfg.ClusterName, "", "")

	expectMessage := "{\"msg_type\": \"interactive\",\"card\": {\"config\": {\"wide_screen_mode\": true},\"header\": {\"title\": {\"tag\": \"plain_text\",\"content\": \"\"},\"template\": \"blue\"},\"elements\": [{\"tag\":\"markdown\",\"content\":\"There is an issue with container in a pod!\\n**Cluster:** dev\\n**Pod:** test-pod\\n**Container:** test-container\\n**Namespace:** default\\n**Reason:** OOMKILLED\\n**Events:**\\n```\\ntest\\n```\\n**Logs:**\\n```\\ntest\\ntestlogs\\n```\"}]}}"

	assertions.Equal(expectMessage, c.buildRequestBodyFeiShu(formattedMsg))
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
	c := NewFeiShu(configMap, &config.App{ClusterName: "dev"})
	assertions.NotNil(c)

	assertions.Nil(c.SendMessage("test"))
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
	c := NewFeiShu(configMap, &config.App{ClusterName: "dev"})
	assertions.NotNil(c)

	assertions.NotNil(c.SendMessage("test"))
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
	c := NewFeiShu(configMap, &config.App{ClusterName: "dev"})
	assertions.NotNil(c)

	ev := event.Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		Reason:        "OOMKILLED",
		Logs:          "test\ntestlogs",
		Events: "event1-event2-event3-event1-event2-event3-event1-event2-" +
			"event3\nevent5\nevent6-event8-event11-event12",
	}
	assertions.Nil(c.SendEvent(&ev))
}

func TestInvalidHttpRequest(t *testing.T) {
	assertions := assert.New(t)

	configMap := map[string]interface{}{
		"webhook": "h ttp://localhost",
	}
	c := NewFeiShu(configMap, &config.App{ClusterName: "dev"})
	assertions.NotNil(c)

	assertions.NotNil(c.SendMessage("test"))

	configMap = map[string]interface{}{
		"webhook": "http://localhost:132323",
	}
	c = NewFeiShu(configMap, &config.App{ClusterName: "dev"})
	assertions.NotNil(c)

	assertions.NotNil(c.SendMessage("test"))
}

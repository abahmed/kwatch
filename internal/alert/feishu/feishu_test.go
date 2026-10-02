package feishu

import (
	"context"
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
	assertions := assert.New(t)

	c := NewFeiShu(map[string]interface{}{}, testAppConfig(), testDeps)
	assertions.Nil(c)
}

func TestRocketChat(t *testing.T) {
	assertions := assert.New(t)

	configMap := map[string]interface{}{
		"webhook": "https://example.test/hook",
	}
	c := NewFeiShu(configMap, testAppConfig(), testDeps)
	assertions.NotNil(c)

	assertions.Equal(c.Name(), "Fei Shu")
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

func TestSendMessageReportsAPIErrorBody(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"code":999,"msg":"invalid signature"}`))
		},
	))
	defer s.Close()

	c := NewFeiShu(
		map[string]interface{}{"webhook": s.URL},
		testAppConfig(), testDeps,
	)
	assert.Error(t, c.SendMessage(context.Background(), "test"))
}

func TestInvalidHttpRequest(t *testing.T) {
	assertions := assert.New(t)

	configMap := map[string]interface{}{
		"webhook": "https://example.test/hook",
	}
	c := NewFeiShu(configMap, testAppConfig(), testDeps)
	assertions.NotNil(c)
	c.webhook = "h ttp://localhost"

	assertions.NotNil(c.SendMessage(context.Background(), "test"))

	configMap = map[string]interface{}{
		"webhook": "http://localhost:132323",
	}
	c = NewFeiShu(configMap, testAppConfig(), testDeps)
	assertions.NotNil(c)

	assertions.NotNil(c.SendMessage(context.Background(), "test"))
}

package wecom

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

	c := NewWecom(map[string]interface{}{}, testAppConfig(), testDeps)
	assert.Nil(c)
}

func TestWecom(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"webhook": "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=test",
	}
	c := NewWecom(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	assert.Equal(c.Name(), "WeCom")
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	var gotBody string
	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			w.Write([]byte(`{"errcode":0}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"webhook": s.URL,
	}
	c := NewWecom(configMap, testAppConfig(), testDeps)

	assert.Nil(c.SendMessage(context.Background(), "hello"))
	assert.Contains(gotBody, `"msgtype":"markdown"`)
	assert.Contains(gotBody, `"content":"hello"`)
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"webhook": s.URL,
	}
	c := NewWecom(configMap, testAppConfig(), testDeps)

	assert.NotNil(c.SendMessage(context.Background(), "test"))
}

func TestSendMessageReportsAPIErrorBody(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"errcode":40001,"errmsg":"invalid token"}`))
		},
	))
	defer s.Close()

	c := NewWecom(
		map[string]interface{}{"webhook": s.URL},
		testAppConfig(), testDeps,
	)
	assert.Error(t, c.SendMessage(context.Background(), "test"))
}

func TestSendIncident(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			assert.Contains(string(body), "crash-looping")
			w.Write([]byte(`{"errcode":0}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"webhook": s.URL,
	}
	c := NewWecom(configMap, testAppConfig(), testDeps)

	assert.Nil(c.SendIncident(context.Background(), providertest.Announce()))
}

func TestInvalidHttpRequest(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"webhook": "https://example.test/hook",
	}
	c := NewWecom(configMap, testAppConfig(), testDeps)
	c.webhook = "h ttp://localhost"

	assert.NotNil(c.SendMessage(context.Background(), "test"))
}

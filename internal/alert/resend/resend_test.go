package resend

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

	c := NewResend(map[string]interface{}{}, testAppConfig(), testDeps)
	assert.Nil(c)
}

func TestResend(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"apiKey": "re_test",
		"from":   "kwatch@example.com",
		"to":     "ops@example.com",
	}
	c := NewResend(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	assert.Equal(c.Name(), "Resend")
	assert.Equal(c.url, "https://api.resend.com/emails")
}

func TestResendMultiTo(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"apiKey": "re_test",
		"from":   "kwatch@example.com",
		"to":     "ops@example.com, dev@example.com",
	}
	c := NewResend(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	assert.Len(c.to, 2)
}

func TestResendInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewResend(
		map[string]interface{}{
			"from": "f",
			"to":   "t",
		},
		testAppConfig(),
		testDeps,
	)
	assert.Nil(c)

	c = NewResend(
		map[string]interface{}{
			"apiKey": "a",
			"to":     "t",
		},
		testAppConfig(),
		testDeps,
	)
	assert.Nil(c)

	c = NewResend(
		map[string]interface{}{
			"apiKey": "a",
			"from":   "f",
		},
		testAppConfig(),
		testDeps,
	)
	assert.Nil(c)
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	var gotAuth string
	var gotBody string
	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			w.WriteHeader(http.StatusOK)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"apiKey":  "re_test",
		"from":    "kwatch@example.com",
		"to":      "ops@example.com",
		"subject": "kwatch alert",
	}
	c := NewResend(configMap, testAppConfig(), testDeps)
	c.url = s.URL

	assert.Nil(c.SendMessage(context.Background(), "hello"))
	assert.Equal("Bearer re_test", gotAuth)
	assert.Contains(gotBody, `"from":"kwatch@example.com"`)
	assert.Contains(gotBody, `"to":["ops@example.com"]`)
	assert.Contains(gotBody, `"subject":"kwatch alert"`)
	assert.Contains(gotBody, `"text":"hello"`)
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"apiKey": "re_test",
		"from":   "kwatch@example.com",
		"to":     "ops@example.com",
	}
	c := NewResend(configMap, testAppConfig(), testDeps)
	c.url = s.URL

	assert.NotNil(c.SendMessage(context.Background(), "test"))
}

func TestSendEvent(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			assert.Contains(string(body), "OOMKILLED")
			w.WriteHeader(http.StatusOK)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"apiKey": "re_test",
		"from":   "kwatch@example.com",
		"to":     "ops@example.com",
	}
	c := NewResend(configMap, testAppConfig(), testDeps)
	c.url = s.URL

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
		"apiKey": "re_test",
		"from":   "kwatch@example.com",
		"to":     "ops@example.com",
	}
	c := NewResend(configMap, testAppConfig(), testDeps)
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage(context.Background(), "test"))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendMessage(context.Background(), "test"))
}

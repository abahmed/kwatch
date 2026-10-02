package dingtalk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestSendMessageNetworkError(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"accessToken": "testToken",
	}
	c := NewDingTalk(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	c.url = "http://localhost:99999/send"

	err := c.SendMessage(context.Background(), "test")
	assert.NotNil(err)
}

func TestSendIncidentNetworkError(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"accessToken": "testToken",
	}
	c := NewDingTalk(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	c.url = "http://localhost:99999/send"

	err := c.SendIncident(context.Background(), providertest.Announce())
	assert.NotNil(err)
}

func TestSendMessageErrorResponseStatus(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"errcode": 400, "errmsg": "bad request"}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"accessToken": "testToken",
	}
	c := NewDingTalk(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	c.url = s.URL + "/send?accessToken=%s"

	err := c.SendMessage(context.Background(), "test")
	assert.NotNil(err)
}

func TestSendMessageWithInvalidUTF8(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"accessToken": "testToken",
	}
	c := NewDingTalk(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	c.url = "http://localhost:99999"

	invalidUTF8 := string([]byte{0xff, 0xfe})
	err := c.SendMessage(context.Background(), invalidUTF8)
	assert.NotNil(err)
}

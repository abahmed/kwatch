package mattermost

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSendMessageNeutralizesBroadcastMention(t *testing.T) {
	assert := assert.New(t)

	var gotBody string
	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			w.Write([]byte(`{"isOk": true}`))
		}))
	defer s.Close()

	configMap := map[string]interface{}{
		"webhook": s.URL,
	}
	c := NewMattermost(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)

	assert.Nil(c.SendMessage(
		context.Background(), "heads up @channel please look",
	))
	assert.True(strings.Contains(gotBody, "@\u200bchannel"))
}

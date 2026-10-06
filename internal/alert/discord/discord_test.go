package discord

import (
	"context"
	"errors"
	"net/http"
	"testing"

	discordgo "github.com/bwmarrin/discordgo"
	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

func mockedSend(
	webhookID,
	token string,
	wait bool,
	data *discordgo.WebhookParams,
	options ...discordgo.RequestOption) (st *discordgo.Message, err error) {
	return nil, nil
}

func newTestDiscord(
	values map[string]interface{}, clusterName string,
) *Discord {
	return NewDiscord(values, clusterName, transport.Dependencies{
		HTTPClient: http.DefaultClient,
		Clock:      clock.RealClock{},
	})
}

func TestDiscordEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	c := newTestDiscord(map[string]interface{}{}, "dev")
	assert.Nil(c)
}

func TestDiscordInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"webhook": "testtest",
	}
	c := newTestDiscord(configMap, "dev")
	assert.Nil(c)
}

func TestDiscord(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"webhook": "test/test",
	}
	c := newTestDiscord(configMap, "dev")
	assert.NotNil(c)

	assert.Equal(c.Name(), "Discord")
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"webhook": "test/test",
	}
	c := newTestDiscord(configMap, "dev")
	assert.NotNil(c)

	c.send = mockedSend
	assert.Nil(c.SendMessage(context.Background(), "test"))
}

func TestDiscordHTTPClientErrorsAreClassified(t *testing.T) {
	err := &discordgo.RESTError{
		Response: &http.Response{
			StatusCode: http.StatusBadRequest,
			Status:     "400 Bad Request",
		},
	}
	classified := wrapDiscordRateLimit(err)
	assert.True(t, transport.IsPermanent(classified))

	transient := &discordgo.RESTError{
		Response: &http.Response{
			StatusCode: http.StatusBadGateway,
			Status:     "502 Bad Gateway",
		},
	}
	assert.False(t, transport.IsPermanent(wrapDiscordRateLimit(transient)))
	assert.False(t, transport.IsPermanent(
		wrapDiscordRateLimit(errors.New("network"))))
}

func TestVerifyUsesConfiguredHostAndThread(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewDiscord(map[string]interface{}{
		"webhook": rec.URL() + "/api/webhooks/123/tok?thread_id=77",
	}, "dev", rec.Dependencies())
	if c == nil {
		t.Fatal("discord was not constructed")
	}
	if err := c.Verify(context.Background()); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	last := rec.Last(t)
	if last.Method != "GET" || last.Path != "/api/webhooks/123/tok" ||
		last.Query != "thread_id=77" {
		t.Fatalf("verify request = %s %s?%s",
			last.Method, last.Path, last.Query)
	}
}

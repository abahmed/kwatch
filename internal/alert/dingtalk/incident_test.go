package dingtalk

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func recorderDingTalk(
	t *testing.T, title string,
) (*DingTalk, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	c := NewDingTalk(map[string]interface{}{
		"accessToken": "secret", "title": title,
	}, "dev", rec.Dependencies())
	require.NotNil(t, c)
	c.url = rec.URL() + "/robot/send?access_token=%s"
	return c, rec
}

func TestDingTalkSendIncidentRendersNote(t *testing.T) {
	c, rec := recorderDingTalk(t, "")
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			m := tc.Message
			require.NoError(t, c.SendIncident(context.Background(), m))
			req := rec.Last(t)
			assert.Equal(t, "access_token=secret", req.Query)
			body := req.JSON(t)
			assert.Equal(t, "markdown", body["msgtype"])
			markdown := body["markdown"].(map[string]any)
			assert.Equal(t, m.Short, markdown["title"])
			text := markdown["text"].(string)
			assert.True(t, strings.HasPrefix(text, m.Note))
			providertest.AssertOneLeadingEmoji(t, text)
		})
	}
}

func TestDingTalkSendIncidentUsesConfiguredTitle(t *testing.T) {
	c, rec := recorderDingTalk(t, "kwatch")
	require.NoError(t, c.SendIncident(context.Background(),
		providertest.Announce()))
	markdown := rec.Last(t).JSON(t)["markdown"].(map[string]any)
	assert.Equal(t, "kwatch", markdown["title"])
}

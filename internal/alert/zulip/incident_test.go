package zulip

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestZulipSendIncidentRendersNote(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewZulip(map[string]interface{}{
		"email": "kwatch@example.com", "token": "secret",
		"channel": "alerts", "url": rec.URL(),
	}, "dev", rec.Dependencies())
	require.NotNil(t, c)
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			m := tc.Message
			require.NoError(t, c.SendIncident(context.Background(), m))
			req := rec.Last(t)
			assert.Equal(t, "/api/v1/messages", req.Path)
			form, err := url.ParseQuery(string(req.Body))
			require.NoError(t, err)
			assert.Equal(t, "alerts", form.Get("to"))
			assert.Equal(t, "kwatch alert", form.Get("subject"))
			content := form.Get("content")
			assert.True(t, strings.HasPrefix(content, m.Note))
			providertest.AssertOneLeadingEmoji(t, content)
			if len(m.Output) > 0 {
				assert.Contains(t, content, "```\npanic: out of memory\n```")
			}
		})
	}
}

func TestZulipSendIncidentNeutralizesMentions(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewZulip(map[string]interface{}{
		"email": "kwatch@example.com", "token": "secret",
		"channel": "alerts", "url": rec.URL(),
	}, "dev", rec.Dependencies())
	require.NotNil(t, c)
	m := providertest.Announce()
	m.Output = []string{"@**all** @**everyone** @_**Ann**"}
	require.NoError(t, c.SendIncident(context.Background(), m))
	form, err := url.ParseQuery(string(rec.Last(t).Body))
	require.NoError(t, err)
	content := form.Get("content")
	for _, mention := range []string{"@**all**", "@**everyone**", "@_**"} {
		assert.NotContains(t, content, mention)
	}
}

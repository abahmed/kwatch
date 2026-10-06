package zulip

import (
	"context"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestZulipFencesOutputAndBreaksGroupMentions(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewZulip(map[string]interface{}{
		"url": rec.URL(), "email": "bot@example.com", "token": "k",
		"channel": "ops",
	}, "dev", rec.Dependencies())
	require.NotNil(t, c)

	m := providertest.Announce()
	m.Note += " @*admins*"
	m.Output = []string{"``` @_*oncall*"}
	require.NoError(t, c.SendIncident(context.Background(), m))

	form, err := url.ParseQuery(string(rec.Last(t).Body))
	require.NoError(t, err)
	content := form.Get("content")
	assert.Contains(t, content, "\n````\n``` ")
	assert.NotContains(t, content, "@*")
	assert.NotContains(t, content, "@_*")
}

package matrix

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestMatrixSendIncidentNeutralizesRoomMention(t *testing.T) {
	c, rec := recorderMatrix(t)
	m := providertest.Hostile()
	m.Note = "ping @room now"
	m.Output = []string{"log says @room and @channel"}
	require.NoError(t, c.SendIncident(context.Background(), m))
	body := rec.Last(t).JSON(t)
	for _, key := range []string{"body", "formatted_body"} {
		text := body[key].(string)
		assert.False(t, strings.Contains(text, "@room"), key)
		assert.False(t, strings.Contains(text, "@channel"), key)
		assert.Contains(t, text, "@​room", key)
	}
}

func TestMatrixSendMessageNeutralizesRoomMention(t *testing.T) {
	c, rec := recorderMatrix(t)
	require.NoError(t, c.SendMessage(context.Background(), "hi @room"))
	body := rec.Last(t).JSON(t)
	assert.NotContains(t, body["body"].(string), "@room")
}

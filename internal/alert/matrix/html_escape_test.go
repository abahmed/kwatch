package matrix

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/transport"
)

type captureSender struct{ body []byte }

func (c *captureSender) Send(
	_ context.Context, r transport.Request,
) ([]byte, error) {
	c.body = r.Body
	return nil, nil
}

func TestMatrixSendMessageEscapesHTML(t *testing.T) {
	sender := &captureSender{}
	m := &Matrix{sender: sender, homeServer: "https://m.example"}
	err := m.SendMessage(
		context.Background(), "log <img src=x onerror=alert(1)>\n<nil>",
	)
	assert.NoError(t, err)
	var payload map[string]string
	assert.NoError(t, json.Unmarshal(sender.body, &payload))
	assert.Equal(t, "log <img src=x onerror=alert(1)>\n<nil>", payload["body"])
	assert.Equal(t,
		"log &lt;img src=x onerror=alert(1)&gt;<br/>&lt;nil&gt;",
		payload["formatted_body"],
	)
}

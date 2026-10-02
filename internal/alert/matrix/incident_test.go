package matrix

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

func recorderMatrix(t *testing.T) (*Matrix, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	c := NewMatrix(map[string]interface{}{
		"homeServer": rec.URL(), "accessToken": "secret",
		"internalRoomId": "!room:example",
	}, "dev", rec.Dependencies())
	require.NotNil(t, c)
	return c, rec
}

func TestMatrixSendIncidentRendersNote(t *testing.T) {
	c, rec := recorderMatrix(t)
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			m := tc.Message
			require.NoError(t, c.SendIncident(context.Background(), m))
			req := rec.Last(t)
			body := req.JSON(t)
			assert.Equal(t, http.MethodPut, req.Method)
			assert.Equal(t, "Bearer secret", req.Header.Get("Authorization"))
			assert.Equal(t, "m.text", body["msgtype"])
			plain := body["body"].(string)
			assert.True(t, strings.HasPrefix(plain, m.Note))
			providertest.AssertOneLeadingEmoji(t, plain)
			formatted := body["formatted_body"].(string)
			providertest.AssertOneLeadingEmoji(t, formatted)
			if len(m.Output) > 0 {
				assert.Contains(t, formatted,
					"<pre><code>panic: out of memory</code></pre>")
			}
		})
	}
}

func TestMatrixSendIncidentEscapesHTML(t *testing.T) {
	c, rec := recorderMatrix(t)
	m := providertest.Hostile()
	m.Output = []string{"<img src=x onerror=alert(1)>"}
	require.NoError(t, c.SendIncident(context.Background(), m))
	body := rec.Last(t).JSON(t)
	formatted := body["formatted_body"].(string)
	assert.NotContains(t, formatted, "<script>")
	assert.NotContains(t, formatted, "<b>")
	assert.NotContains(t, formatted, "<img")
	assert.Contains(t, formatted, "&lt;script&gt;")
	assert.Contains(t, body["body"].(string), "<script>")
}

func TestMatrixSendIncidentKeepsDeliveredNote(t *testing.T) {
	c, rec := recorderMatrix(t)
	m := providertest.Announce()
	m.Output = nil
	m.Note = notification.Truncate(m.Note+strings.Repeat("x", 500), 100)
	require.NoError(t, c.SendIncident(context.Background(), m))
	assert.Equal(t, m.Note, rec.Last(t).JSON(t)["body"])
}

func TestMatrixErrorCodeInSuccessBodyFails(t *testing.T) {
	c, rec := recorderMatrix(t)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		_, _ = w.Write([]byte(`{"errcode":"M_FORBIDDEN"}`))
	}
	err := c.SendIncident(context.Background(), providertest.Announce())
	require.Error(t, err)
	assert.True(t, transport.IsPermanent(err))
}

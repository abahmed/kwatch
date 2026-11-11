package vonage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func sentForm(t *testing.T, text string) url.Values {
	t.Helper()
	var got string
	s := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			got = string(body)
			_, _ = w.Write([]byte(`{"messages":[{"status":"0"}]}`))
		}))
	defer s.Close()
	c := NewVonage(map[string]interface{}{
		"apiKey": "k", "apiSecret": "s", "from": "kwatch",
		"to": "+12025550100",
	}, "dev", testDeps)
	c.url = s.URL
	assert.NoError(t, c.SendMessage(context.Background(), text))
	form, err := url.ParseQuery(got)
	assert.NoError(t, err)
	return form
}

// The status emoji is not GSM-7: without type=unicode the handset shows
// replacement characters.
func TestEmojiTextIsSentAsUnicode(t *testing.T) {
	form := sentForm(t, "🔴 payments is crash-looping")
	assert.Equal(t, "unicode", form.Get("type"))
}

func TestGsm7TextKeepsTheDefaultType(t *testing.T) {
	form := sentForm(t, "Café £5 {ok} €1")
	assert.Empty(t, form.Get("type"))
}

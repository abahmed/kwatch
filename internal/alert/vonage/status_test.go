package vonage

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/ratelimit"
)

func sendWithStatus(t *testing.T, status string) error {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(
				`{"messages":[{"status":"` + status + `"}]}`))
		}))
	defer s.Close()
	c := NewVonage(map[string]interface{}{
		"apiKey": "k", "apiSecret": "s", "from": "kwatch",
		"to": "+12025550100",
	}, testAppConfig(), testDeps)
	c.url = s.URL
	return c.SendMessage(context.Background(), "hi")
}

func TestStatusOneIsARateLimit(t *testing.T) {
	err := sendWithStatus(t, "1")
	var limited *ratelimit.Error
	assert.True(t, errors.As(err, &limited))
	assert.False(t, transport.IsPermanent(err))
}

func TestCredentialAndParameterStatusesArePermanent(t *testing.T) {
	for _, status := range []string{"2", "3", "4", "9", "15"} {
		assert.True(t,
			transport.IsPermanent(sendWithStatus(t, status)), status)
	}
}

func TestServerSideStatusesRetry(t *testing.T) {
	for _, status := range []string{"5", "13"} {
		err := sendWithStatus(t, status)
		assert.Error(t, err)
		assert.False(t, transport.IsPermanent(err), status)
	}
}

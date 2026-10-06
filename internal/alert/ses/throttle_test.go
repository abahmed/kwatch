package ses

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

func sesAnswering(t *testing.T, code int, body string) error {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(body))
		}))
	defer s.Close()
	c := NewSes(map[string]interface{}{
		"accessKeyId": "AKIA123", "secretAccessKey": "x",
		"from": "kwatch@example.com", "to": "ops@example.com",
	}, testAppConfig(), testDeps)
	c.url = s.URL
	return c.SendMessage(context.Background(), "hi")
}

func TestThrottling400IsARateLimit(t *testing.T) {
	err := sesAnswering(t, 400, "<ErrorResponse><Error><Type>Sender</Type>"+
		"<Code>Throttling</Code><Message>Maximum sending rate exceeded."+
		"</Message></Error></ErrorResponse>")
	var limited *ratelimit.Error
	assert.True(t, errors.As(err, &limited))
	assert.False(t, transport.IsPermanent(err))
}

func TestOtherBadRequestStaysPermanent(t *testing.T) {
	err := sesAnswering(t, 400, "<Error><Code>MessageRejected</Code></Error>")
	assert.True(t, transport.IsPermanent(err))
}

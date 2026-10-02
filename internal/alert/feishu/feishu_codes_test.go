package feishu

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/ratelimit"
)

func TestFeiShuBodyErrorClassification(t *testing.T) {
	tests := []struct {
		name        string
		code        int
		permanent   bool
		rateLimited bool
	}{
		{"revoked webhook is permanent", 19001, true, false},
		{"unrelated 9499 is permanent", 9499, true, false},
		{"frequency limit is rate limited", 11232, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, _ *http.Request) {
					_, _ = fmt.Fprintf(w, `{"code": %d}`, tc.code)
				}))
			defer s.Close()
			c := NewFeiShu(map[string]interface{}{
				"webhook": s.URL,
			}, testAppConfig(), testDeps)

			err := c.SendMessage(context.Background(), "test")
			assert.Error(t, err)
			assert.Equal(t, tc.permanent, transport.IsPermanent(err))
			var rl *ratelimit.Error
			assert.Equal(t, tc.rateLimited, errors.As(err, &rl))
		})
	}
}

package dingtalk

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

func TestDingTalkBodyErrorClassification(t *testing.T) {
	tests := []struct {
		name        string
		code        int
		permanent   bool
		rateLimited bool
	}{
		{"rejected message is permanent", 310000, true, false},
		{"frequency limit is rate limited", 130101, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, _ *http.Request) {
					_, _ = fmt.Fprintf(w, `{"errcode": %d}`, tc.code)
				}))
			defer s.Close()
			c := NewDingTalk(map[string]interface{}{
				"accessToken": "testToken",
			}, testAppConfig(), testDeps)
			c.url = s.URL + "/send?accessToken=%s"

			err := c.SendMessage(context.Background(), "test")
			assert.Error(t, err)
			assert.Equal(t, tc.permanent, transport.IsPermanent(err))
			var rl *ratelimit.Error
			assert.Equal(t, tc.rateLimited, errors.As(err, &rl))
		})
	}
}

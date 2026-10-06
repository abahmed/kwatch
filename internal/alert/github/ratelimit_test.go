package github

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/ratelimit"
)

func forbidden(t *testing.T, message string) *Github {
	t.Helper()
	rec := providertest.NewRecorder(t)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"` + message + `"}`))
	}
	return NewGithub(map[string]interface{}{
		"token": "t", "owner": "o", "repo": "r", "url": rec.URL(),
	}, "dev", rec.Dependencies())
}

func TestGithubRateLimited403IsRetryable(t *testing.T) {
	for _, message := range []string{
		"API rate limit exceeded for user",
		"You have exceeded a secondary rate limit",
	} {
		g := forbidden(t, message)
		_, err := g.Create(context.Background(), "t", "b")
		var limited *ratelimit.Error
		assert.True(t, errors.As(err, &limited), message)
		assert.False(t, transport.IsPermanent(err), message)
	}
}

func TestGithubOther403StaysPermanent(t *testing.T) {
	g := forbidden(t, "Resource not accessible by integration")
	_, err := g.Create(context.Background(), "t", "b")
	assert.Error(t, err)
	assert.True(t, transport.IsPermanent(err))
}

func TestGithubRateLimitHonoursAWaitNamedInTheMessage(t *testing.T) {
	g := forbidden(t, "Secondary rate limit. Retry after 90 seconds.")
	_, err := g.Create(context.Background(), "t", "b")
	var limited *ratelimit.Error
	if assert.True(t, errors.As(err, &limited)) {
		assert.Equal(t, 90*time.Second, limited.RetryAfter)
	}
}

func TestGithubWaitHintIsCappedAndOptional(t *testing.T) {
	assert.Equal(t, maxWaitHint,
		waitHint(nil, []byte("rate limit, wait 120 minutes")))
	assert.Zero(t, waitHint(nil, []byte("rate limit exceeded")))
}

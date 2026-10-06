package matrix

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/ratelimit"
)

func matrixOn(rec *providertest.Recorder, server string) *Matrix {
	return NewMatrix(map[string]interface{}{
		"homeServer": server, "accessToken": "t", "internalRoomId": "!r:x",
	}, "dev", rec.Dependencies())
}

func TestMatrix429UsesRetryAfterMs(t *testing.T) {
	rec := providertest.NewRecorder(t)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(
			`{"errcode":"M_LIMIT_EXCEEDED","retry_after_ms":2500}`))
	}
	err := matrixOn(rec, rec.URL()).SendMessage(context.Background(), "hi")

	var limited *ratelimit.Error
	require.True(t, errors.As(err, &limited))
	assert.Equal(t, 2500*time.Millisecond, limited.RetryAfter)
}

func TestMatrixHomeServerTrailingSlashIsTrimmed(t *testing.T) {
	rec := providertest.NewRecorder(t)
	err := matrixOn(rec, rec.URL()+"/").SendMessage(context.Background(), "hi")
	require.NoError(t, err)
	assert.NotContains(t, rec.Last(t).Path, "//")
}

package health

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHealthServerLivenessCheckFailsHealthz(t *testing.T) {
	h := &HealthServer{}
	healthz := func() int {
		rec := httptest.NewRecorder()
		h.healthzHandler(rec, httptest.NewRequest(http.MethodGet,
			"/healthz", nil))
		return rec.Code
	}

	assert.Equal(t, http.StatusOK, healthz())
	h.SetLivenessCheck(func() error { return errors.New("hung") })
	assert.Equal(t, http.StatusServiceUnavailable, healthz())
	h.SetLivenessCheck(nil)
	assert.Equal(t, http.StatusOK, healthz())
}

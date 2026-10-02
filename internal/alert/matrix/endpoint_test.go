package matrix

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestMatrixRejectsInvalidHomeServer(t *testing.T) {
	rec := providertest.NewRecorder(t)
	build := func(endpoint string) *Matrix {
		return NewMatrix(map[string]interface{}{
			"homeServer":     endpoint,
			"accessToken":    "t",
			"internalRoomId": "!r:x",
		}, "dev", rec.Dependencies())
	}
	for _, endpoint := range []string{"not a url", "ftp://x", "/relative"} {
		assert.Nil(t, build(endpoint), endpoint)
	}
	assert.NotNil(t, build(rec.URL()))
}

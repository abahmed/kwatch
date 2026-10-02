package n8n

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestN8nRejectsInvalidURL(t *testing.T) {
	rec := providertest.NewRecorder(t)
	build := func(endpoint string) *N8n {
		return NewN8n(map[string]interface{}{
			"url": endpoint,
		}, "dev", rec.Dependencies())
	}
	for _, endpoint := range []string{"not a url", "ftp://x", "/relative"} {
		assert.Nil(t, build(endpoint), endpoint)
	}
	assert.NotNil(t, build(rec.URL()))
}

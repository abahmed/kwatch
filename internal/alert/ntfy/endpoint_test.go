package ntfy

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestNtfyRejectsInvalidURL(t *testing.T) {
	rec := providertest.NewRecorder(t)
	build := func(endpoint string) *Ntfy {
		return NewNtfy(map[string]interface{}{
			"url":   endpoint,
			"topic": "alerts",
		}, "dev", rec.Dependencies())
	}
	for _, endpoint := range []string{"not a url", "ftp://x", "/relative"} {
		assert.Nil(t, build(endpoint), endpoint)
	}
	assert.NotNil(t, build(rec.URL()))
	assert.NotNil(t, build(""), "unset uses the default")
}

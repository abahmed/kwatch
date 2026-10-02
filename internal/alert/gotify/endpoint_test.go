package gotify

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestGotifyRejectsInvalidURL(t *testing.T) {
	rec := providertest.NewRecorder(t)
	build := func(endpoint string) *Gotify {
		return NewGotify(map[string]interface{}{
			"url":   endpoint,
			"token": "t",
		}, "dev", rec.Dependencies())
	}
	for _, endpoint := range []string{"not a url", "ftp://x", "/relative"} {
		assert.Nil(t, build(endpoint), endpoint)
	}
	assert.NotNil(t, build(rec.URL()))
}

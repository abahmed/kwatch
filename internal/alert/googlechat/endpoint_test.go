package googlechat

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestGoogleChatRejectsInvalidWebhook(t *testing.T) {
	rec := providertest.NewRecorder(t)
	build := func(endpoint string) *GoogleChat {
		return NewGoogleChat(map[string]interface{}{
			"webhook": endpoint,
		}, "dev", rec.Dependencies())
	}
	for _, endpoint := range []string{"not a url", "ftp://x", "/relative"} {
		assert.Nil(t, build(endpoint), endpoint)
	}
	assert.NotNil(t, build(rec.URL()))
}

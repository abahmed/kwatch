package webex

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A message goes to one destination: both roomId and toPersonEmail make
// Webex refuse every send, so the config is rejected at startup.
func TestWebexRejectsBothDestinations(t *testing.T) {
	c := NewWebex(map[string]interface{}{
		"accessToken": "t", "roomId": "r", "toPersonEmail": "a@b.c",
	}, testAppConfig(), testDeps)
	assert.Nil(t, c)
}

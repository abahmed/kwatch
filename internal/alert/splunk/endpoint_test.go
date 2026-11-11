package splunk

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestSplunkURLWithoutPathGetsTheEventEndpoint(t *testing.T) {
	deps := providertest.NewRecorder(t).Dependencies()
	for url, want := range map[string]string{
		"https://splunk.corp:8088":    "https://splunk.corp:8088" + eventPath,
		"https://splunk.corp:8088/":   "https://splunk.corp:8088" + eventPath,
		"https://splunk.corp/custom":  "https://splunk.corp/custom",
		"https://splunk.corp/a/event": "https://splunk.corp/a/event",
	} {
		c := NewSplunk(map[string]interface{}{
			"url": url, "token": "t"}, "dev", deps)
		require.NotNil(t, c)
		assert.Equal(t, want, c.url)
	}
}

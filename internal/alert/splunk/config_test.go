package splunk

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestSplunkConfig(t *testing.T) {
	deps := providertest.NewRecorder(t).Dependencies()
	c := NewSplunk(map[string]interface{}{
		"url": "https://splunk.example.com:8088", "token": "t",
	}, "dev", deps)
	assert.Equal(t, "Splunk", c.Name())
	assert.Nil(t, NewSplunk(map[string]interface{}{"token": "t"}, "dev",
		deps))
	assert.Nil(t, NewSplunk(map[string]interface{}{"url": "https://a.test"}, "dev",
		deps))
}

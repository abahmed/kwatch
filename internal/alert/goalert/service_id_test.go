package goalert

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// serviceId is never sent, so a config without it must still start.
func TestGoalertServiceIDIsOptional(t *testing.T) {
	c := NewGoalert(map[string]interface{}{
		"token": "tok", "url": "https://goalert.corp",
	}, testAppConfig(), testDeps)
	assert.NotNil(t, c)
}

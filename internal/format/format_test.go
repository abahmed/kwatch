package format

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDuration(t *testing.T) {
	assert.Equal(t, "5s", Duration(5e9))
	assert.Equal(t, "2m30s", Duration(150e9))
	assert.Equal(t, "2h15m", Duration(8100e9))
	assert.Equal(t, "1m", Duration(60e9))
	assert.Equal(t, "1h", Duration(3600e9))
	assert.Equal(t, "2h", Duration(7200e9))
}

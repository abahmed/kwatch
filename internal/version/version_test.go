package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShort(t *testing.T) {
	assert := assert.New(t)

	result := Short()
	assert.Equal("dev", result)
}

func TestVersionConstants(t *testing.T) {
	assert := assert.New(t)

	assert.Equal("dev", version)
	assert.Equal("none", gitCommitID)
	assert.Equal("unknown", buildDate)
}

func TestShortMultipleCalls(t *testing.T) {
	assert := assert.New(t)

	result1 := Short()
	result2 := Short()

	assert.Equal(result1, result2)
	assert.Equal("dev", result1)
	assert.Equal("dev", result2)
}



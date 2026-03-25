package version

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShort(t *testing.T) {
	assert := assert.New(t)

	result := Short()
	assert.NotEmpty(result)
	assert.Equal("dev", result)
}

func TestVersion(t *testing.T) {
	assert := assert.New(t)

	result := Version()
	assert.NotEmpty(result)

	var info Info
	err := json.Unmarshal([]byte(result), &info)
	assert.Nil(err)
	assert.Equal("dev", info.Version)
	assert.Equal("none", info.GitCommit)
	assert.Equal("unknown", info.BuildDate)
}

func TestVersionConstants(t *testing.T) {
	assert := assert.New(t)

	assert.Equal("dev", version)
	assert.Equal("none", gitCommitID)
	assert.Equal("unknown", buildDate)
}

func TestInfoStruct(t *testing.T) {
	assert := assert.New(t)

	info := Info{
		Version:   "v0.10.0",
		GitCommit: "abc123",
		BuildDate: "2024-01-01",
	}

	assert.Equal("v0.10.0", info.Version)
	assert.Equal("abc123", info.GitCommit)
	assert.Equal("2024-01-01", info.BuildDate)
}

func TestShortMultipleCalls(t *testing.T) {
	assert := assert.New(t)

	result1 := Short()
	result2 := Short()

	assert.Equal(result1, result2)
	assert.Equal("dev", result1)
	assert.Equal("dev", result2)
}

func TestVersionMultipleCalls(t *testing.T) {
	assert := assert.New(t)

	result1 := Version()
	result2 := Version()

	assert.Equal(result1, result2)
}

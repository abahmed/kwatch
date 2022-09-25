package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
)

func TestGetAllowForbidSlices(t *testing.T) {
	assert := assert.New(t)

	testCases := []map[string][]string{
		{
			"input":  {},
			"allow":  {},
			"forbid": {},
		},
		{
			"input":  {"hello", "!world"},
			"allow":  {"hello"},
			"forbid": {"world"},
		},
		{
			"input":  {"hello"},
			"allow":  {"hello"},
			"forbid": {},
		},
		{
			"input":  {"!hello"},
			"allow":  {},
			"forbid": {"hello"},
		},
	}

	for _, tc := range testCases {
		actualAllow, actualForbid := getAllowForbidSlices(tc["input"])
		assert.Equal(actualAllow, tc["allow"])
		assert.Equal(actualForbid, tc["forbid"])
	}
}

func TestConfig(t *testing.T) {
	assert := assert.New(t)

	cfg, _ := LoadConfig()
	assert.NotNil(cfg)
}

func TestConfigFromFile(t *testing.T) {
	assert := assert.New(t)

	defer os.Unsetenv("CONFIG_FILE")
	defer os.RemoveAll("config.yaml")

	os.Setenv("CONFIG_FILE", "config.yaml")

	n := Config{
		MaxRecentLogLines: 20,
		Namespaces:        []string{"default", "!kwatch"},
		Reasons:           []string{"default", "!kwatch"},
	}
	yamlData, _ := yaml.Marshal(&n)
	os.WriteFile("config.yaml", yamlData, 0644)

	cfg, _ := LoadConfig()
	assert.NotNil(cfg)
	assert.Equal(cfg.MaxRecentLogLines, 20)
	assert.Len(cfg.AllowedNamespaces, 1)
	assert.Len(cfg.AllowedReasons, 1)
	assert.Len(cfg.ForbiddenNamespaces, 1)
	assert.Len(cfg.ForbiddenReasons, 1)

	os.WriteFile("config.yaml", []byte("maxRecentLogLines: test"), 0644)
	_, err := LoadConfig()
	assert.NotNil(err)
}

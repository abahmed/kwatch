package config

import (
	"fmt"
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

	c, _ := LoadConfig()
	assert.NotNil(c)
}
func TestConfigFromFile(t *testing.T) {
	defer os.Unsetenv("CONFIG_FILE")
	defer os.RemoveAll("config.yaml")

	os.Setenv("CONFIG_FILE", "config.yaml")

	n := Config{
		MaxRecentLogLines: 10,
		Namespaces:        []string{"default", "!kwatch"},
	}
	yamlData, _ := yaml.Marshal(&n)
	os.WriteFile("config.yaml", yamlData, 0644)
	LoadConfig()

	n = Config{
		MaxRecentLogLines: 10,
		Reasons:           []string{"default", "!kwatch"},
	}
	yamlData, _ = yaml.Marshal(&n)
	w := string(yamlData)
	fmt.Println(w)
	os.WriteFile("config.yaml", yamlData, 0644)
	LoadConfig()

	os.WriteFile("config.yaml", []byte("maxRecentLogLines: test"), 0644)
	LoadConfig()
}

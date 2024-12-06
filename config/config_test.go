package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
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

func TestEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	os.Setenv("CONFIG_FILE", "config.yaml")
	defer os.Unsetenv("CONFIG_FILE")

	os.WriteFile("config.yaml", []byte{}, 0644)
	defer os.RemoveAll("config.yaml")

	cfg, _ := LoadConfig()
	assert.NotNil(cfg)
}

func TestConfigInvalidFile(t *testing.T) {
	assert := assert.New(t)
	cfg, err := LoadConfig()
	assert.Nil(cfg)
	assert.NotNil(err)
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
		IgnorePodNames:    []string{"my-fancy-pod-[.*"},
		IgnoreLogPatterns: []string{"leaderelection lost-[.*"},
		App: App{
			ProxyURL:    "https://localhost",
			ClusterName: "development",
		},
	}
	yamlData, _ := yaml.Marshal(&n)
	os.WriteFile("config.yaml", yamlData, 0644)

	cfg, _ := LoadConfig()
	assert.NotNil(cfg)

	assert.Equal(cfg.App.ClusterName, "development")
	assert.Equal(cfg.App.ProxyURL, "https://localhost")

	assert.Equal(cfg.MaxRecentLogLines, int64(20))
	assert.Len(cfg.AllowedNamespaces, 1)
	assert.Len(cfg.AllowedReasons, 1)
	assert.Len(cfg.ForbiddenNamespaces, 1)
	assert.Len(cfg.ForbiddenReasons, 1)

	os.WriteFile("config.yaml", []byte("maxRecentLogLines: test"), 0644)
	_, err := LoadConfig()
	assert.NotNil(err)
}

func TestGetCompiledIgnorePatterns(t *testing.T) {
	assert := assert.New(t)

	validPatterns := []string{
		"my-fancy-pod-[0-9]",
		"leaderelection lost",
	}

	compiledPatterns, err := getCompiledIgnorePatterns(validPatterns)

	assert.Nil(err)
	assert.True(compiledPatterns[0].MatchString("my-fancy-pod-8"))
	assert.True(compiledPatterns[1].MatchString(`controllermanager.go:272] "leaderelection lost"`))

	invalidPatterns := []string{
		"my-fancy-pod-[.*",
	}

	compiledPatterns, err = getCompiledIgnorePatterns(invalidPatterns)

	assert.NotNil(err)
}

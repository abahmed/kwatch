package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
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
	assert.Equal(int64(50), cfg.MaxRecentLogLines)
	assert.Equal(600, cfg.ResyncSeconds)
	assert.Equal(true, cfg.PendingPodMonitor.Enabled)
	assert.Equal(true, cfg.RolloutMonitor.Enabled)
	assert.Equal(true, cfg.JobMonitor.Enabled)
	assert.Equal(true, cfg.CronJobMonitor.Enabled)
	assert.Equal(true, cfg.DaemonSetMonitor.Enabled)
	assert.Equal(true, cfg.HpaMonitor.Enabled)
	assert.Equal(true, cfg.HealthCheck.Enabled)
	assert.Equal(8060, cfg.HealthCheck.Port)
}

func TestConfigInvalidFile(t *testing.T) {
	assert := assert.New(t)

	os.Setenv("CONFIG_FILE", "bad-config.yaml")
	defer os.Unsetenv("CONFIG_FILE")

	os.WriteFile("bad-config.yaml", []byte("test"), 0644)
	defer os.RemoveAll("bad-config.yaml")

	cfg, err := LoadConfig()
	assert.Nil(cfg)
	assert.NotNil(err)
}

func TestConfigFromFile(t *testing.T) {
	assert := assert.New(t)

	defer os.Unsetenv("CONFIG_FILE")
	defer os.RemoveAll("config.yaml")

	os.Setenv("CONFIG_FILE", "config.yaml")

	yamlContent := `
maxRecentLogLines: 20
namespaces:
  - default
  - kwatch
reasons:
  - CrashLoopBackOff
  - OOMKilling
ignorePodNames:
  - my-fancy-pod-.*
ignoreLogPatterns:
  - leader-election-.*
app:
  proxyURL: https://localhost
  clusterName: development
`
	os.WriteFile("config.yaml", []byte(yamlContent), 0644)

	cfg, err := LoadConfig()
	assert.Nil(err)
	assert.NotNil(cfg)

	assert.Equal(cfg.App.ClusterName, "development")
	assert.Equal(cfg.App.ProxyURL, "https://localhost")

	assert.Equal(cfg.MaxRecentLogLines, int64(20))
	assert.Len(cfg.AllowedNamespaces, 2)
	assert.Len(cfg.AllowedReasons, 2)
	assert.Len(cfg.ForbiddenNamespaces, 0)
	assert.Len(cfg.ForbiddenReasons, 0)

	os.WriteFile("config.yaml", []byte("maxRecentLogLines: test"), 0644)
	_, err = LoadConfig()
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

func TestConfigEnvInterpolation(t *testing.T) {
	assert := assert.New(t)

	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.yaml"
	t.Setenv("CONFIG_FILE", configPath)

	t.Setenv("TEST_WEBHOOK", "https://hooks.example.com/x")
	t.Setenv("TEST_MISSING", "")
	t.Setenv("A", "hello")

	// YAML with ${VAR}, literal $, and bare $VAR
	content := []byte(`
app:
  clusterName: "${TEST_WEBHOOK}"
  proxyURL: "$HOME"
namespaces:
  - "${TEST_MISSING}"
reasons:
  - "pass$2a$10$xyz"
`)
	os.WriteFile(configPath, content, 0644)

	cfg, err := LoadConfig()
	assert.Nil(err)
	assert.NotNil(cfg)

	// ${TEST_WEBHOOK} → expanded
	assert.Equal("https://hooks.example.com/x", cfg.App.ClusterName)

	// bare $HOME → NOT expanded (left as literal)
	assert.Equal("$HOME", cfg.App.ProxyURL)

	// ${TEST_MISSING} (unset) → empty string
	assert.Len(cfg.AllowedNamespaces, 1)
	assert.Equal("", cfg.AllowedNamespaces[0])

	// literal $ in bcrypt-like value → unchanged
	assert.Len(cfg.AllowedReasons, 1)
	assert.Equal("pass$2a$10$xyz", cfg.AllowedReasons[0])

	// verify mixed {A}-$B case
	os.WriteFile(configPath, []byte(`app:
  clusterName: "${A}-$B"
`), 0644)
	cfg2, err2 := LoadConfig()
	assert.Nil(err2)
	assert.NotNil(cfg2)
	assert.Equal("hello-$B", cfg2.App.ClusterName)
}

func TestIgnoreNodeReasonsLoading(t *testing.T) {
	assert := assert.New(t)

	defer os.Unsetenv("CONFIG_FILE")
	defer os.RemoveAll("config.yaml")

	os.Setenv("CONFIG_FILE", "config.yaml")

	os.WriteFile("config.yaml", []byte(`
ignoreNodeReasons:
  - NotReady
  - KubeletNotReady
  - custom-reason
`), 0644)

	cfg, err := LoadConfig()
	assert.Nil(err)
	assert.NotNil(cfg)
	assert.Equal([]string{"NotReady", "KubeletNotReady", "custom-reason"}, cfg.IgnoreNodeReasons)
}

func TestIgnoreNodeReasonsEmpty(t *testing.T) {
	assert := assert.New(t)

	defer os.Unsetenv("CONFIG_FILE")
	defer os.RemoveAll("config.yaml")

	os.Setenv("CONFIG_FILE", "config.yaml")

	os.WriteFile("config.yaml", []byte(`
ignoreNodeReasons: []
`), 0644)

	cfg, err := LoadConfig()
	assert.Nil(err)
	assert.NotNil(cfg)
	assert.Equal([]string{}, cfg.IgnoreNodeReasons)
}

func TestIgnoreNodeReasonsSpecialChars(t *testing.T) {
	assert := assert.New(t)

	defer os.Unsetenv("CONFIG_FILE")
	defer os.RemoveAll("config.yaml")

	os.Setenv("CONFIG_FILE", "config.yaml")

	os.WriteFile("config.yaml", []byte(`
ignoreNodeReasons:
  - reason-1
  - reason_2
  - reason.with.dot
  - reason/with/slash
`), 0644)

	cfg, err := LoadConfig()
	assert.Nil(err)
	assert.NotNil(cfg)
	assert.Equal([]string{"reason-1", "reason_2", "reason.with.dot", "reason/with/slash"}, cfg.IgnoreNodeReasons)
}

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

func TestValidateNamespaceEntries(t *testing.T) {
	for _, entries := range [][]string{{""}, {"!"}, {"UPPER"}, {"has space"}} {
		if errs := validateNamespaceEntries(entries); len(errs) == 0 {
			t.Fatalf("expected invalid namespace error for %q", entries)
		}
	}
	if errs := validateNamespaceEntries(
		[]string{"default", "!kube-system"},
	); len(errs) != 0 {
		t.Fatalf("valid namespaces rejected: %v", errs)
	}
}

func TestValidateReasonEntriesRejectsWhitespace(t *testing.T) {
	errs := validateReasonEntries([]string{" ", "! ", "CrashLoopBackOff"})
	assert.Len(t, errs, 2)
}

func TestEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.yaml"
	t.Setenv("CONFIG_FILE", configPath)

	os.WriteFile(configPath, []byte{}, 0644)

	cfg, _ := LoadConfig()
	assert.NotNil(cfg)
	assert.Equal(int64(50), cfg.MaxRecentLogLines)
	assert.Equal(0, cfg.ResyncSeconds)
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

	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.yaml"
	t.Setenv("CONFIG_FILE", configPath)

	os.WriteFile(configPath, []byte("test"), 0644)

	cfg, err := LoadConfig()
	assert.Nil(cfg)
	assert.NotNil(err)
}

func TestConfigFromFile(t *testing.T) {
	assert := assert.New(t)

	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.yaml"
	t.Setenv("CONFIG_FILE", configPath)

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
	os.WriteFile(configPath, []byte(yamlContent), 0644)

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

	os.WriteFile(configPath, []byte("maxRecentLogLines: test"), 0644)
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
	assert.Empty(compiledPatterns)
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
	assert.ErrorContains(err, "namespaces entries must not be empty")
	assert.Nil(cfg)

	// verify mixed {A}-$B case
	os.WriteFile(configPath, []byte(`app:
  clusterName: "${A}-$B"
`), 0644)
	cfg2, err2 := LoadConfig()
	assert.Nil(err2)
	assert.NotNil(cfg2)
	assert.Equal("hello-$B", cfg2.App.ClusterName)
}

func TestConfigEnvInterpolationUnsetVarErrors(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.yaml"
	t.Setenv("CONFIG_FILE", configPath)

	// A truly unset variable must fail loudly instead of silently
	// expanding to an empty string.
	os.WriteFile(configPath, []byte(`app:
  clusterName: "${TEST_UNSET_VAR}"
`), 0644)
	cfg, err := LoadConfig()
	assert.Nil(t, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TEST_UNSET_VAR")
}

func testConfigFile(t *testing.T, content string) (*Config, error) {
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.yaml"
	t.Setenv("CONFIG_FILE", configPath)
	os.WriteFile(configPath, []byte(content), 0644)
	return LoadConfig()
}

func TestIgnoreNodeReasonsLoading(t *testing.T) {
	cfg, err := testConfigFile(t, `
ignoreNodeReasons:
  - NotReady
  - KubeletNotReady
  - custom-reason
`)
	assert.Nil(t, err)
	assert.NotNil(t, cfg)
	assert.Equal(t, []string{"NotReady", "KubeletNotReady", "custom-reason"}, cfg.IgnoreNodeReasons)
}

func TestIgnoreNodeReasonsEmpty(t *testing.T) {
	cfg, err := testConfigFile(t, `
ignoreNodeReasons: []
`)
	assert.Nil(t, err)
	assert.NotNil(t, cfg)
	assert.Equal(t, []string{}, cfg.IgnoreNodeReasons)
}

func TestIgnoreNodeReasonsSpecialChars(t *testing.T) {
	cfg, err := testConfigFile(t, `
ignoreNodeReasons:
  - reason-1
  - reason_2
  - reason.with.dot
  - reason/with/slash
`)
	assert.Nil(t, err)
	assert.NotNil(t, cfg)
	assert.Equal(t, []string{"reason-1", "reason_2", "reason.with.dot", "reason/with/slash"}, cfg.IgnoreNodeReasons)
}

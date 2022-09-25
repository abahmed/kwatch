package config

import (
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type Config struct {
	// MaxRecentLogLines optional max tail log lines in messages,
	// if it's not provided it will get all log lines
	MaxRecentLogLines int `mapstructure:"maxRecentLogLines"`

	// IgnoreFailedGracefulShutdown if set to true, containers which are
	// forcefully killed during shutdown (as their graceful shutdown failed)
	// are not reported as error
	IgnoreFailedGracefulShutdown bool `mapstructure:"ignoreFailedGracefulShutdown"`

	// DisableUpdateCheck if set to true, does not check for and
	// notify about kwatch updates
	DisableUpdateCheck bool `mapstructure:"DisableUpdateCheck"`

	// Namespaces is an optional list of namespaces that you want to watch or
	// forbid, if it's not provided it will watch all namespaces.
	// If you want to forbid a namespace, configure it with !<namespace name>
	// You can either set forbidden namespaces or allowed, not both
	Namespaces []string `mapstructure:"namespaces"`

	// Reasons is an  optional list of reasons that you want to watch or forbid,
	// if it's not provided it will watch all reasons.
	// If you want to forbid a reason, configure it with !<reason>
	// You can either set forbidden reasons or allowed, not both
	Reasons []string `mapstructure:"reasons"`

	// IgnoreContainerNames optional list of container names to ignore
	IgnoreContainerNames []string `mapstructure:"ignoreContainerNames"`

	// Alert is a map contains a map of each provider configuration
	// e.g. {"slack": {"webhook": "URL"}}
	Alert map[string]map[string]string `mapstructure:"alert"`

	Upgrader Upgrader `mapstructure:"upgrader"`

	// AllowedNamespaces, ForbiddenNamespaces are calculated internally
	// after loading Namespaces configuration
	AllowedNamespaces   []string
	ForbiddenNamespaces []string

	// AllowedReasons, ForbiddenReasons are calculated internally after loading
	// Reasons configuration
	AllowedReasons   []string
	ForbiddenReasons []string
}

type Upgrader struct {
	// MaxRecentLogLines optional max tail log lines in messages,
	// if it's not provided it will get all log lines
	CheckInterval int `mapstructure:"checkInterval"`

	// DisableUpdateCheck if set to true, does not check for and
	// notify about kwatch updates
	DisableUpdateCheck bool `mapstructure:"DisableUpdateCheck"`
}

// LoadConfig loads yaml configuration from file if provided, otherwise
// loads default configuration
func LoadConfig() (*Config, error) {
	// initialize configuration
	configFile := os.Getenv("CONFIG_FILE")
	if len(configFile) != 0 {
		viper.SetConfigFile(configFile)
	}
	viper.AutomaticEnv()

	// if a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		logrus.Infof("using config file: %s", viper.ConfigFileUsed())
	} else {
		logrus.Warnf("unable to load config file: %s", err.Error())
	}

	q := viper.AllSettings()
	logrus.Infof("%v", q)
	// Load config
	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, err
	}

	// Parse namespace allow/forbid lists
	config.AllowedNamespaces, config.ForbiddenNamespaces =
		getAllowForbidSlices(config.Namespaces)
	if len(config.AllowedNamespaces) > 0 &&
		len(config.ForbiddenNamespaces) > 0 {
		logrus.Error(
			"Either allowed or forbidden namespaces must be set. " +
				"Can't set both")
	}

	// Parse reason allow/forbid lists
	config.AllowedReasons, config.ForbiddenReasons =
		getAllowForbidSlices(config.Reasons)
	if len(config.AllowedReasons) > 0 &&
		len(config.ForbiddenReasons) > 0 {
		logrus.Error("Either allowed or forbidden reasons must be set. " +
			"Can't set both")
	}

	// Should be removed
	config.Upgrader.DisableUpdateCheck = config.DisableUpdateCheck

	return &config, nil
}

// getAllowForbidSlices split input slice into two slices by items start with !
func getAllowForbidSlices(items []string) (allow []string, forbid []string) {
	allow = make([]string, 0)
	forbid = make([]string, 0)
	for _, item := range items {
		if clean := strings.TrimPrefix(item, "!"); item != clean {
			forbid = append(forbid, clean)
			continue
		}
		allow = append(allow, item)
	}
	return allow, forbid
}

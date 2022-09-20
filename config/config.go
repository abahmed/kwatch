package config

import (
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type Config struct {
	MaxRecentLogLines            int                          `mapstructure:"maxRecentLogLines"`
	IgnoreFailedGracefulShutdown bool                         `mapstructure:"ignoreFailedGracefulShutdown"`
	DisableUpdateCheck           bool                         `mapstructure:"DisableUpdateCheck"`
	Namespaces                   []string                     `mapstructure:"namespaces"`
	Reasons                      []string                     `mapstructure:"reasons"`
	IgnoreContainerNames         []string                     `mapstructure:"ignoreContainerNames"`
	Alert                        map[string]map[string]string `mapstructure:"alert"`

	AllowedNamespaces   []string
	AllowedReasons      []string
	ForbiddenNamespaces []string
	ForbiddenReasons    []string
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
		logrus.Fatal(
			"Either allowed or forbidden namespaces must be set. " +
				"Can't set both")
	}

	// Parse reason allow/forbid lists
	config.AllowedReasons, config.ForbiddenReasons =
		getAllowForbidSlices(config.Reasons)
	if len(config.AllowedReasons) > 0 &&
		len(config.ForbiddenReasons) > 0 {
		logrus.Fatal("Either allowed or forbidden reasons must be set. " +
			"Can't set both")
	}

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

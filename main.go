package main

import (
	"os"
	"path/filepath"

	"github.com/abahmed/kwatch/controller"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

func main() {
	// initialize configuration
	configFile := os.Getenv("CONFIG_FILE")
	if len(configFile) == 0 {
		configFile = filepath.Join("config.yaml")
	}
	viper.SetConfigFile(configFile)
	viper.AutomaticEnv()

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		logrus.Infof("using config file: %s", viper.ConfigFileUsed())
	} else {
		logrus.Warnf("unable to load config file: %s", err.Error())
	}

	// start controller
	controller.Start()
}

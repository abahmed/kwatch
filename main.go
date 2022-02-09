package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/abahmed/kwatch/constant"
	"github.com/abahmed/kwatch/controller"
	"github.com/abahmed/kwatch/upgrader"
	"github.com/abahmed/kwatch/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

func main() {
	logrus.Infof(fmt.Sprintf(constant.WelcomeMsg, constant.Version))

	// initialize configuration
	configFile := os.Getenv("CONFIG_FILE")
	if len(configFile) == 0 {
		configFile = filepath.Join("config.yaml")
	}
	viper.SetConfigFile(configFile)
	viper.AutomaticEnv()

	// if a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		logrus.Infof("using config file: %s", viper.ConfigFileUsed())
	} else {
		logrus.Warnf("unable to load config file: %s", err.Error())
	}

	// get providers
	providers := util.GetProviders()

	// check and notify if newer versions are available
	if !viper.GetBool("disableUpdateCheck") {
		go upgrader.CheckUpdates(providers)
	}

	// start controller
	controller.Start(providers, viper.GetBool("ignoreFailedGracefulShutdown"))
}

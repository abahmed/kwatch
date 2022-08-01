package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	// Parse namespace allow/forbid lists
	namespaceAllowList := make([]string, 0)
	namespaceForbidList := make([]string, 0)
	for _, namespace := range viper.GetStringSlice("namespaces") {
		if clean := strings.TrimPrefix(namespace, "!"); namespace != clean {
			namespaceForbidList = append(namespaceForbidList, clean)
			continue
		}
		namespaceAllowList = append(namespaceAllowList, namespace)
	}
	if len(namespaceAllowList) > 0 && len(namespaceForbidList) > 0 {
		logrus.Fatal("Either allowed or forbidden namespaces must be set. Can't set both")
	}

	// Parse reason allow/forbid lists
	reasonAllowList := make([]string, 0)
	reasonForbidList := make([]string, 0)
	for _, namespace := range viper.GetStringSlice("reasons") {
		if clean := strings.TrimPrefix(namespace, "!"); namespace != clean {
			reasonForbidList = append(reasonForbidList, clean)
			continue
		}
		reasonAllowList = append(reasonAllowList, namespace)
	}
	if len(reasonAllowList) > 0 && len(reasonForbidList) > 0 {
		logrus.Fatal("Either allowed or forbidden reasons must be set. Can't set both")
	}

	igonreContainerList := make([]string, 0)
	for _, containr := range viper.GetStringSlice("ignoreContainrNames") {
		igonreContainerList = append(igonreContainerList, containr)
	}

	// start controller
	controller.Start(providers, viper.GetBool("ignoreFailedGracefulShutdown"), namespaceAllowList, namespaceForbidList, reasonAllowList, reasonForbidList, igonreContainerList)
}

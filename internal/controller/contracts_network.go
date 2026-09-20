package controller

import networkmonitor "github.com/abahmed/kwatch/internal/monitor/network"

type NetworkProcessor interface {
	ProcessService(string, bool) error
	ProcessNetworkPolicy(string, bool) error
	ProcessIngress(string, bool) error
}

type NetworkConfig interface {
	ConfigureSources(networkmonitor.Sources) error
}

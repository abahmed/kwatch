package config

type Config struct {
	// App general configuration
	App App `yaml:"app"`

	// Upgrader configuration
	Upgrader Upgrader `yaml:"upgrader"`

	// PvcMonitor configuration
	PvcMonitor PvcMonitor `yaml:"pvcMonitor"`

	// MaxRecentLogLines optional max tail log lines in messages,
	// if it's not provided it will get all log lines
	MaxRecentLogLines int64 `yaml:"maxRecentLogLines"`

	// IgnoreFailedGracefulShutdown if set to true, containers which are
	// forcefully killed during shutdown (as their graceful shutdown failed)
	// are not reported as error
	IgnoreFailedGracefulShutdown bool `yaml:"ignoreFailedGracefulShutdown"`

	// Namespaces is an optional list of namespaces that you want to watch or
	// forbid, if it's not provided it will watch all namespaces.
	// If you want to forbid a namespace, configure it with !<namespace name>
	// You can either set forbidden namespaces or allowed, not both
	Namespaces []string `yaml:"namespaces"`

	// Reasons is an  optional list of reasons that you want to watch or forbid,
	// if it's not provided it will watch all reasons.
	// If you want to forbid a reason, configure it with !<reason>
	// You can either set forbidden reasons or allowed, not both
	Reasons []string `yaml:"reasons"`

	// IgnoreContainerNames optional list of container names to ignore
	IgnoreContainerNames []string `yaml:"ignoreContainerNames"`

	// Alert is a map contains a map of each provider configuration
	// e.g. {"slack": {"webhook": "URL"}}
	Alert map[string]map[string]string `yaml:"alert"`

	// AllowedNamespaces, ForbiddenNamespaces are calculated internally
	// after loading Namespaces configuration
	AllowedNamespaces   []string
	ForbiddenNamespaces []string

	// AllowedReasons, ForbiddenReasons are calculated internally after loading
	// Reasons configuration
	AllowedReasons   []string
	ForbiddenReasons []string
}

// App confing struct
type App struct {
	// ProxyURL to be used in outgoing http(s) requests except Kubernetes
	// requests to cluster
	ProxyURL string `yaml:"proxyURL"`

	// ClusterName to used in notifications to indicate which cluster has
	// issue
	ClusterName string `yaml:"clusterName"`
}

// Upgrader confing struct
type Upgrader struct {
	// DisableUpdateCheck if set to true, does not check for and
	// notify about kwatch updates
	DisableUpdateCheck bool `yaml:"disableUpdateCheck"`
}

// PvcMonitor confing struct
type PvcMonitor struct {
	// Enabled if set to true, it will check pvc usage periodically
	// By default, this value is true
	Enabled bool `yaml:"enabled"`

	// Interval is the frequency (in minutes) to check pvc usage in the cluster
	// By default, this value is 5
	Interval int `yaml:"interval"`

	// Threshold is the percentage of accepted pvc usage. if current usage
	// exceeds this value, it will send a notification.
	// By default, this value is 80
	Threshold float64 `yaml:"threshold"`
}

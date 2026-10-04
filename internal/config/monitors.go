package config

// MaintenanceConfig describes annotations used to silence deliberate pod
// maintenance without disabling monitoring for the rest of the cluster.
type MaintenanceConfig struct {
	Enabled         bool   `yaml:"enabled"`
	Annotation      string `yaml:"annotation"`
	UntilAnnotation string `yaml:"untilAnnotation"`
}

// HeartbeatMonitor config for dead man's switch
type HeartbeatMonitor struct {
	// Enabled if set to true, a periodic heartbeat ping is sent.
	Enabled bool `yaml:"enabled"`

	// Interval is the frequency (in seconds) between pings. Default 300 (5 min).
	Interval int `yaml:"interval"`

	// URL is the external endpoint to ping (e.g. Healthchecks.io).
	// When set, a GET request is sent every interval; no response means the
	// external monitor pages.
	URL string `yaml:"url"`
}

// ActiveProbeMonitor performs checks against configured endpoints and, when
// AutoServices is enabled, advertised Service ports from inside kwatch.
type ActiveProbeMonitor struct {
	Enabled          bool              `yaml:"enabled"`
	IntervalSeconds  int               `yaml:"intervalSeconds"`
	TimeoutSeconds   int               `yaml:"timeoutSeconds"`
	FailureThreshold int               `yaml:"failureThreshold"`
	HTTP             []HTTPProbeTarget `yaml:"http"`
	TCP              []TCPProbeTarget  `yaml:"tcp"`
	DNS              []DNSProbeTarget  `yaml:"dns"`
	// AutoServices probes every Service port in scope from kwatch's own pod.
	// That is a real TCP connection from a real pod, so a NetworkPolicy that
	// does not admit kwatch reports the Service as down when it is fine.
	AutoServices bool `yaml:"autoServices"`
	// ExcludeNamespaces are namespaces AutoServices skips entirely. Use it
	// for namespaces with default-deny ingress that do not admit kwatch.
	ExcludeNamespaces []string `yaml:"excludeNamespaces"`
	// AutoDependencies probes, over TCP from kwatch's own pod, every
	// endpoint outside the cluster that a pod's environment names (a
	// database URL, a queue address). A dependency that refuses
	// connections then explains the pods that call it.
	AutoDependencies bool `yaml:"autoDependencies"`
}

// HTTPProbeTarget is one HTTP endpoint the probe monitor checks.
type HTTPProbeTarget struct {
	Name              string `yaml:"name"`
	URL               string `yaml:"url"`
	ExpectedStatus    int    `yaml:"expectedStatus"`
	LatencyWarningMs  int    `yaml:"latencyWarningMs"`
	LatencyCriticalMs int    `yaml:"latencyCriticalMs"`
}

// TCPProbeTarget is one TCP address the probe monitor dials.
type TCPProbeTarget struct {
	Name    string `yaml:"name"`
	Address string `yaml:"address"`
}

// DNSProbeTarget is one DNS name the probe monitor resolves.
type DNSProbeTarget struct {
	Name string `yaml:"name"`
	Host string `yaml:"host"`
}

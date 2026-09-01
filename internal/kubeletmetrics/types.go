package kubeletmetrics

import "time"

type persistedState struct {
	Previous  map[string]metricSnapshot `json:"previous"`
	Failures  map[string]int            `json:"failures"`
	Successes map[string]int            `json:"successes"`
	StateSeen map[string]time.Time      `json:"stateSeen"`
	Baselines map[string]usageBaseline  `json:"baselines,omitempty"`
}

type endpointStatus struct {
	Summary, CAdvisor, Runtime bool
	RBACDenied                 bool
}

type Status struct {
	State             string    `json:"state"`
	LastSweep         time.Time `json:"lastSweep"`
	Nodes             int       `json:"nodes"`
	SummaryAvailable  int       `json:"summaryAvailable"`
	CAdvisorAvailable int       `json:"cadvisorAvailable"`
	RuntimeAvailable  int       `json:"runtimeAvailable"`
	RBACDenied        int       `json:"rbacDenied"`
}

type metricSnapshot struct {
	At        time.Time
	Throttled float64
	Periods   float64
	RxErrors  uint64
	TxErrors  uint64
}

type usageBaseline struct {
	Percent float64   `json:"percent"`
	Samples int       `json:"samples"`
	M2      float64   `json:"m2,omitempty"`
	Updated time.Time `json:"updated"`
}

// The following types mirror the kubelet stats/summary response. They are
// deliberately kept private because the endpoint schema is an implementation
// detail of this monitor.
type summary struct {
	Node nodeSummary  `json:"node"`
	Pods []podSummary `json:"pods"`
}

type podSummary struct {
	PodRef     podReference       `json:"podRef"`
	Containers []containerSummary `json:"containers"`
}

type podReference struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type containerSummary struct {
	Name   string           `json:"name"`
	CPU    *cpuStats        `json:"cpu"`
	Memory *memoryStats     `json:"memory"`
	RootFS *filesystemStats `json:"rootfs"`
}

type cpuStats struct {
	UsageNanoCores uint64 `json:"usageNanoCores"`
}

type memoryStats struct {
	WorkingSetBytes uint64 `json:"workingSetBytes"`
}

type filesystemStats struct {
	UsedBytes uint64 `json:"usedBytes"`
}

type nodeSummary struct {
	CPU     resourceStats `json:"cpu"`
	Memory  resourceStats `json:"memory"`
	Network *networkStats `json:"network"`
}

type resourceStats struct {
	PSI *psiStats `json:"psi"`
}

type psiStats struct {
	Some psiWindow `json:"some"`
	Full psiWindow `json:"full"`
}

type psiWindow struct {
	Avg10  float64 `json:"avg10"`
	Avg60  float64 `json:"avg60"`
	Avg300 float64 `json:"avg300"`
}

type networkStats struct {
	RxErrors uint64 `json:"rxErrors"`
	TxErrors uint64 `json:"txErrors"`
}

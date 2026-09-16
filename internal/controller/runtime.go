package controller

import (
	"time"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	clustermonitor "github.com/abahmed/kwatch/internal/monitor/cluster"
	workloadmonitor "github.com/abahmed/kwatch/internal/monitor/workload"
)

// RuntimeDependencies are the controller's non-informer dependencies. They
// are supplied at construction so graph, change-tracking, and readiness state
// cannot be replaced after workers begin processing events.
type RuntimeDependencies struct {
	Tracker *kwcontext.ChangeTracker
	Graph   *kwcontext.ResourceGraph
	Ready   func()
	Now     func() time.Time
}

// RuntimeSet contains the family runtimes assembled by the application. The
// controller owns this composition boundary because it dispatches queues and
// wires informer-backed sources into each family.
type RuntimeSet struct {
	IncidentSources IncidentSourceConfig
	Pod             PodRuntime
	Workload        WorkloadRuntime
	Node            NodeRuntime
	Network         NetworkRuntime
	Security        SecurityRuntime
	Cluster         ClusterRuntime
	Integration     IntegrationRuntime
	Baseline        BaselineSink
}

type PodRuntime struct {
	Processor PodProcessor
	Config    PodConfig
}

type WorkloadRuntime struct {
	Deployments  DeploymentProcessor
	DaemonSets   DaemonSetProcessor
	StatefulSets StatefulSetProcessor
	CronJobs     CronJobProcessor
	HPAs         HPAProcessor
	PDBs         PDBProcessor
	ReplicaSets  ReplicaSetProcessor
	Jobs         JobProcessor
	SourceConfig workloadmonitor.SourceConfig
}

type NodeRuntime struct {
	Processor NodeProcessor
	Config    NodeConfig
}

type NetworkRuntime struct {
	Processor NetworkProcessor
	Config    NetworkConfig
}

type SecurityRuntime struct {
	Processor SecurityProcessor
	Config    SecurityConfig
}

type ClusterRuntime struct {
	Processor ResourceProcessor
	Config    clustermonitor.SourceConfig
}

type IntegrationRuntime struct {
	ControlPlane       ControlPlaneProcessor
	ControlPlaneConfig ControlPlaneConfig
	TLS                TLSProcessor
	TLSConfig          TLSConfig
	Events             EventProcessor
}

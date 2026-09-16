package workload

import (
	"fmt"
	"sync"

	appsv1lister "k8s.io/client-go/listers/apps/v1"
	autoscalingv2lister "k8s.io/client-go/listers/autoscaling/v2"
	batchv1lister "k8s.io/client-go/listers/batch/v1"
	policyv1lister "k8s.io/client-go/listers/policy/v1"
)

// ReplicaSetConfig is the source-wiring contract for ReplicaSetRuntime.
type ReplicaSetConfig interface {
	configureLister(appsv1lister.ReplicaSetLister)
}

// Sources is the complete informer-backed source view for workload
// processing. Controller wiring supplies it once, after all caches exist.
type Sources struct {
	Deployments  appsv1lister.DeploymentLister
	ReplicaSets  appsv1lister.ReplicaSetLister
	DaemonSets   appsv1lister.DaemonSetLister
	StatefulSets appsv1lister.StatefulSetLister
	Jobs         batchv1lister.JobLister
	CronJobs     batchv1lister.CronJobLister
	HPAs         autoscalingv2lister.HorizontalPodAutoscalerLister
	PDBs         policyv1lister.PodDisruptionBudgetLister
}

// SourceConfig is the one controller-facing source wiring operation for the
// workload family.
type SourceConfig interface {
	ConfigureSources(Sources) error
}

// SourceConfiguration adapts the resource-specific workload runtimes to one
// family source operation. Resource processing remains explicit and separate.
type SourceConfiguration struct {
	mu           sync.Mutex
	configured   bool
	deployments  DeploymentConfig
	replicaSets  ReplicaSetConfig
	daemonSets   DaemonSetConfig
	statefulSets StatefulSetConfig
	jobs         JobConfig
	cronJobs     CronJobConfig
	hpas         HPAConfig
	pdbs         PDBConfig
}

// NewSourceConfiguration constructs workload source wiring for the supplied
// resource runtimes.
func NewSourceConfiguration(
	deployments DeploymentConfig,
	replicaSets ReplicaSetConfig,
	daemonSets DaemonSetConfig,
	statefulSets StatefulSetConfig,
	jobs JobConfig,
	cronJobs CronJobConfig,
	hpas HPAConfig,
	pdbs PDBConfig,
) *SourceConfiguration {
	return &SourceConfiguration{
		deployments: deployments, replicaSets: replicaSets,
		daemonSets: daemonSets, statefulSets: statefulSets,
		jobs: jobs, cronJobs: cronJobs, hpas: hpas, pdbs: pdbs,
	}
}

// ConfigureSources supplies all workload informer caches in one operation.
func (s *SourceConfiguration) ConfigureSources(sources Sources) error {
	if s == nil {
		return fmt.Errorf("workload source configuration is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.configured {
		return fmt.Errorf("workload sources are already configured")
	}
	s.configured = true
	if s.deployments != nil {
		s.deployments.configureLister(sources.Deployments)
	}
	if s.replicaSets != nil {
		s.replicaSets.configureLister(sources.ReplicaSets)
	}
	if s.daemonSets != nil {
		s.daemonSets.configureLister(sources.DaemonSets)
	}
	if s.statefulSets != nil {
		s.statefulSets.configureLister(sources.StatefulSets)
	}
	if s.jobs != nil {
		s.jobs.configureLister(sources.Jobs)
	}
	if s.cronJobs != nil {
		s.cronJobs.configureLister(sources.CronJobs)
	}
	if s.hpas != nil {
		s.hpas.configureLister(sources.HPAs)
	}
	if s.pdbs != nil {
		s.pdbs.configureLister(sources.PDBs)
	}
	return nil
}

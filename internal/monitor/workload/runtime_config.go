package workload

import (
	"time"

	"github.com/abahmed/kwatch/internal/config"
)

// NewDeploymentRuntimeWithRuntimeConfig constructs Deployment monitoring
// from the immutable runtime snapshot.
func NewDeploymentRuntimeWithRuntimeConfig(
	runtime config.RuntimeConfig,
	sink LifecycleSink,
	now func() time.Time,
) *DeploymentRuntime {
	return &DeploymentRuntime{
		support: newRuntimeSupportAt(runtime, sink, now),
	}
}

// NewDaemonSetRuntimeWithRuntimeConfig constructs DaemonSet monitoring from
// the immutable runtime snapshot.
func NewDaemonSetRuntimeWithRuntimeConfig(
	runtime config.RuntimeConfig,
	sink LifecycleSink,
	nodeIncidentCounter NodeIncidentCounter,
	now func() time.Time,
) *DaemonSetRuntime {
	support := newRuntimeSupportAt(runtime, sink, now)
	if nodeIncidentCounter != nil {
		support.activeNodeIncidentCount =
			nodeIncidentCounter.CountActiveNodeIncidents
	}
	return &DaemonSetRuntime{support: support}
}

// NewStatefulSetRuntimeWithRuntimeConfig constructs StatefulSet monitoring
// from the immutable runtime snapshot.
func NewStatefulSetRuntimeWithRuntimeConfig(
	runtime config.RuntimeConfig,
	sink LifecycleSink,
	now func() time.Time,
) *StatefulSetRuntime {
	return &StatefulSetRuntime{
		support: newRuntimeSupportAt(runtime, sink, now),
	}
}

// NewCronJobRuntimeWithRuntimeConfig constructs CronJob monitoring from the
// immutable runtime snapshot.
func NewCronJobRuntimeWithRuntimeConfig(
	runtime config.RuntimeConfig,
	sink LifecycleSink,
	now func() time.Time,
) *CronJobRuntime {
	return &CronJobRuntime{
		support: newRuntimeSupportAt(runtime, sink, now),
	}
}

// NewHPARuntimeWithRuntimeConfig constructs HPA monitoring from the immutable
// runtime snapshot.
func NewHPARuntimeWithRuntimeConfig(
	runtime config.RuntimeConfig,
	sink LifecycleSink,
	now func() time.Time,
) *HPARuntime {
	return &HPARuntime{
		support: newRuntimeSupportAt(runtime, sink, now),
	}
}

// NewPDBRuntimeWithRuntimeConfig constructs PDB monitoring from the
// immutable runtime snapshot.
func NewPDBRuntimeWithRuntimeConfig(
	runtime config.RuntimeConfig,
	sink LifecycleSink,
	now func() time.Time,
) *PDBRuntime {
	return &PDBRuntime{
		support: newRuntimeSupportAt(runtime, sink, now),
	}
}

// NewJobRuntimeWithRuntimeConfig constructs Job monitoring from the immutable
// runtime snapshot.
func NewJobRuntimeWithRuntimeConfig(
	runtime config.RuntimeConfig,
	sink LifecycleSink,
	now func() time.Time,
) *JobRuntime {
	return &JobRuntime{
		support: newRuntimeSupportAt(runtime, sink, now),
	}
}

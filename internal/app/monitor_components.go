package app

import (
	"context"
	"time"

	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/client"
	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/controller"
	"github.com/abahmed/kwatch/internal/controlplane"
	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/kubelet"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/model"
	clustermonitor "github.com/abahmed/kwatch/internal/monitor/cluster"
	networkmonitor "github.com/abahmed/kwatch/internal/monitor/network"
	nodemonitor "github.com/abahmed/kwatch/internal/monitor/node"
	podmonitor "github.com/abahmed/kwatch/internal/monitor/pod"
	securitymonitor "github.com/abahmed/kwatch/internal/monitor/security"
	"github.com/abahmed/kwatch/internal/monitor/workload"
	"github.com/abahmed/kwatch/internal/startup"
)

type monitorComponents struct {
	components      controller.RuntimeSet
	tlsProcessor    controller.TLSProcessor
	tlsConfig       controller.TLSConfig
	controlPlaneRun func(context.Context) error
	controlPlane    health.StatusProvider
	startupSummary  func(map[string]int)
}

func composeMonitorComponents(
	runtime config.RuntimeConfig,
	clientset kubernetes.Interface,
	clients client.ClientSet,
	incidentEngine *incident.Engine,
	deliveryManager *delivery.Manager,
	now func() time.Time,
	startupShouldNotify func() bool,
) monitorComponents {
	podEvaluator := podmonitor.NewPolicyEvaluatorWithRuntimeConfig(
		runtime,
		incidentEngine,
		incidentEngine.GetLastContainerState,
		clock.Func(now),
	)
	podRuntime := podmonitor.NewRuntimeWithRuntimeConfig(
		clientset,
		runtime,
		incidentEngine,
		podEvaluator,
		func(
			ctx context.Context,
			podName, containerName, namespace string,
			previous bool,
			maxLines int64,
		) string {
			return kubelet.GetPodContainerLogs(
				ctx, clientset, podName, containerName,
				namespace, previous, maxLines,
			)
		},
		now,
	)

	nodeRuntime := nodemonitor.NewRuntimeWithRuntimeConfig(
		runtime, incidentEngine, incidentEngine, now,
	)
	networkRuntime := networkmonitor.NewRuntimeWithRuntimeConfig(
		runtime, incidentEngine, now,
	)
	securityRuntime := securitymonitor.NewRuntimeWithRuntimeConfig(
		runtime, incidentEngine,
	)
	clusterRuntime := clustermonitor.NewRuntimeWithRuntimeConfig(
		runtime, incidentEngine, now,
	)
	eventRuntime := clustermonitor.NewEventRuntimeWithRuntimeConfig(
		runtime, incidentEngine, now,
	)

	tlsProcessor, tlsConfig := composeTLSRuntime(
		runtime, incidentEngine, now,
	)
	controlPlaneRun, controlPlaneRuntime := configureControlPlaneMonitor(
		runtime, clientset, clients.REST, clients.Resolver,
		incidentEngine, now,
	)
	integrationRuntime, controlPlaneStatus := composeIntegrationRuntime(
		controlPlaneRuntime, tlsProcessor, tlsConfig, eventRuntime,
	)
	deploymentRuntime := workload.NewDeploymentRuntimeWithRuntimeConfig(
		runtime, incidentEngine, now,
	)
	daemonSetRuntime := workload.NewDaemonSetRuntimeWithRuntimeConfig(
		runtime, incidentEngine, incidentEngine, now,
	)
	statefulSetRuntime := workload.NewStatefulSetRuntimeWithRuntimeConfig(
		runtime, incidentEngine, now,
	)
	cronJobRuntime := workload.NewCronJobRuntimeWithRuntimeConfig(
		runtime, incidentEngine, now,
	)
	hpaRuntime := workload.NewHPARuntimeWithRuntimeConfig(
		runtime, incidentEngine, now,
	)
	pdbRuntime := workload.NewPDBRuntimeWithRuntimeConfig(
		runtime, incidentEngine, now,
	)
	replicaSetRuntime := workload.NewReplicaSetRuntimeWithRuntimeConfig(
		runtime, incidentEngine, now,
	)
	jobRuntime := workload.NewJobRuntimeWithRuntimeConfig(
		runtime, incidentEngine, now,
	)
	workloadSources := workload.NewSourceConfiguration(
		deploymentRuntime, replicaSetRuntime, daemonSetRuntime,
		statefulSetRuntime, jobRuntime, cronJobRuntime, hpaRuntime,
		pdbRuntime,
	)

	return monitorComponents{
		components: controller.RuntimeSet{
			IncidentSources: incidentEngine,
			Pod: controller.PodRuntime{
				Processor: podRuntime, Config: podRuntime,
			},
			Node: controller.NodeRuntime{
				Processor: nodeRuntime, Config: nodeRuntime,
			},
			Workload: controller.WorkloadRuntime{
				Deployments: deploymentRuntime, DaemonSets: daemonSetRuntime,
				StatefulSets: statefulSetRuntime, CronJobs: cronJobRuntime,
				HPAs: hpaRuntime, PDBs: pdbRuntime,
				ReplicaSets: replicaSetRuntime, Jobs: jobRuntime,
				SourceConfig: workloadSources,
			},
			Network: controller.NetworkRuntime{
				Processor: networkRuntime, Config: networkRuntime,
			},
			Security: controller.SecurityRuntime{
				Processor: securityRuntime, Config: securityRuntime,
			},
			Cluster: controller.ClusterRuntime{
				Processor: clusterRuntime, Config: clusterRuntime,
			},
			Integration: integrationRuntime,
			Baseline:    podRuntime,
		},
		controlPlaneRun: controlPlaneRun,
		controlPlane:    controlPlaneStatus,
		tlsProcessor:    tlsProcessor,
		tlsConfig:       tlsConfig,
		startupSummary: func(suppressed map[string]int) {
			if startupShouldNotify != nil && !startupShouldNotify() {
				metrics.DefaultRegistry().StartupSummariesSuppressed.Add(1)
				return
			}
			if inc := startup.BuildSummary(
				runtime.Monitors().ReportStartup(), suppressed,
			); inc != nil {
				deliveryManager.NotifyIncident(
					inc, model.ActionCreate, nil,
				)
			}
		},
	}
}

func composeIntegrationRuntime(
	controlPlane *controlplane.Monitor,
	tlsProcessor controller.TLSProcessor,
	tlsConfig controller.TLSConfig,
	eventRuntime controller.EventProcessor,
) (controller.IntegrationRuntime, health.StatusProvider) {
	integration := controller.IntegrationRuntime{
		TLS:       tlsProcessor,
		TLSConfig: tlsConfig,
		Events:    eventRuntime,
	}
	if controlPlane == nil {
		return integration, nil
	}
	integration.ControlPlane = controlPlane
	integration.ControlPlaneConfig = controlPlane
	return integration, controlPlane
}

func composeTLSRuntime(
	runtime config.RuntimeConfig,
	incidentEngine *incident.Engine,
	now func() time.Time,
) (controller.TLSProcessor, controller.TLSConfig) {
	if !runtime.Monitors().TLS().Enabled {
		return nil, nil
	}
	tlsRuntime := securitymonitor.NewTLSRuntimeWithRuntimeConfig(
		runtime, incidentEngine, now,
	)
	return tlsRuntime, tlsRuntime
}

func newMonitorController(
	clientset kubernetes.Interface,
	runtime config.RuntimeConfig,
	components monitorComponents,
	dependencies controller.RuntimeDependencies,
) (*controller.Controller, func(), error) {
	return controller.NewWithRuntimeConfig(
		clientset,
		runtime,
		components.components,
		dependencies,
	)
}

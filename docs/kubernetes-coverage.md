# Kubernetes coverage

This page is generated from code by `go run ./cmd/coveragedocs`;
do not edit it by hand. It lists which Kubernetes resources kwatch watches
and how, and which failure modes the detectors can raise. For how findings
become incidents, see [architecture](./architecture.md).

## Resources

Watch modes: `full` keeps the whole object, `hashed` keeps a content hash, `status` keeps status only, `metadata` keeps metadata only, `excluded` is not watched.

| Kind | Group | Watch mode |
|:--|:--|:--|
| APIService | apiregistration.k8s.io | status |
| BackendTLSPolicy | gateway.networking.k8s.io | status |
| CSIDriver | storage.k8s.io | metadata |
| CSINode | storage.k8s.io | metadata |
| CSIStorageCapacity | storage.k8s.io | metadata |
| CertificateSigningRequest | certificates.k8s.io | status |
| ClusterRole | rbac.authorization.k8s.io | metadata |
| ClusterRoleBinding | rbac.authorization.k8s.io | metadata |
| ClusterTrustBundle | certificates.k8s.io | metadata |
| ConfigMap | core | hashed |
| ControllerRevision | apps | metadata |
| CronJob | batch | full |
| CustomResourceDefinition | apiextensions.k8s.io | status |
| DaemonSet | apps | full |
| Deployment | apps | full |
| DeviceClass | resource.k8s.io | metadata |
| DeviceTaintRule | resource.k8s.io | status |
| EndpointSlice | discovery.k8s.io | full |
| Endpoints | core | excluded |
| Event | core | full |
| Event | events.k8s.io | excluded |
| FlowSchema | flowcontrol.apiserver.k8s.io | status |
| GRPCRoute | gateway.networking.k8s.io | status |
| Gateway | gateway.networking.k8s.io | status |
| GatewayClass | gateway.networking.k8s.io | status |
| HTTPRoute | gateway.networking.k8s.io | status |
| HorizontalPodAutoscaler | autoscaling | full |
| IPAddress | networking.k8s.io | metadata |
| Ingress | networking.k8s.io | full |
| IngressClass | networking.k8s.io | metadata |
| Job | batch | full |
| KwatchConfig | kwatch.abahmed.dev | excluded |
| Lease | coordination.k8s.io | metadata |
| LeaseCandidate | coordination.k8s.io | metadata |
| LimitRange | core | full |
| ListenerSet | gateway.networking.k8s.io | status |
| MutatingAdmissionPolicy | admissionregistration.k8s.io | status |
| MutatingAdmissionPolicyBinding | admissionregistration.k8s.io | metadata |
| MutatingWebhookConfiguration | admissionregistration.k8s.io | full |
| Namespace | core | full |
| NetworkPolicy | networking.k8s.io | full |
| Node | core | full |
| PersistentVolume | core | full |
| PersistentVolumeClaim | core | full |
| Pod | core | full |
| PodDisruptionBudget | policy | full |
| PodGroup | scheduling.k8s.io | status |
| PodTemplate | core | metadata |
| PriorityClass | scheduling.k8s.io | metadata |
| PriorityLevelConfiguration | flowcontrol.apiserver.k8s.io | status |
| ReferenceGrant | gateway.networking.k8s.io | status |
| ReplicaSet | apps | full |
| ReplicationController | core | status |
| ResourceClaim | resource.k8s.io | status |
| ResourceClaimTemplate | resource.k8s.io | metadata |
| ResourceQuota | core | full |
| ResourceSlice | resource.k8s.io | metadata |
| Role | rbac.authorization.k8s.io | metadata |
| RoleBinding | rbac.authorization.k8s.io | metadata |
| RuntimeClass | node.k8s.io | metadata |
| Secret | core | hashed |
| Service | core | full |
| ServiceAccount | core | full |
| ServiceCIDR | networking.k8s.io | status |
| StatefulSet | apps | full |
| StorageClass | storage.k8s.io | full |
| StorageVersionMigration | storagemigration.k8s.io | status |
| TCPRoute | gateway.networking.k8s.io | status |
| TLSRoute | gateway.networking.k8s.io | status |
| UDPRoute | gateway.networking.k8s.io | status |
| ValidatingAdmissionPolicy | admissionregistration.k8s.io | status |
| ValidatingAdmissionPolicyBinding | admissionregistration.k8s.io | metadata |
| ValidatingWebhookConfiguration | admissionregistration.k8s.io | full |
| VolumeAttachment | storage.k8s.io | full |
| VolumeAttributesClass | storage.k8s.io | metadata |
| VolumeSnapshot | snapshot.storage.k8s.io | status |
| VolumeSnapshotClass | snapshot.storage.k8s.io | status |
| VolumeSnapshotContent | snapshot.storage.k8s.io | status |
| Workload | scheduling.k8s.io | metadata |

## Failure modes

Health is taken from the severity each detector assigns: warning is degraded, critical is failing; `varies` means the severity depends on the observation.

| Mode | Health | Reasons | Detectors |
|:--|:--|:--|:--|
| APIServer.Errors | varies | `APIServerErrors` | `controlplane` |
| APIServer.Throttling | degraded | `APIServerThrottling` | `control_plane_load` |
| APIServiceUnavailable | varies | `APIServiceFailure` | `custom` |
| ActiveProbe | failing | `ActiveProbeFailure` | `active_probe` |
| ActiveProbe.Latency | varies | `ActiveProbeLatency` | `active_probe` |
| Admission.FailingOpen | degraded | `WebhookFailingOpen` | `webhook_calls` |
| Admission.Rejected | failing | `PodAdmissionRejected` | `pod_admission` |
| AttachFailed | failing | `VolumeAttachmentFailure` | `policy` |
| BackendMissing | failing | `IngressBackendNotFound` | `network` |
| BackendsDegraded | degraded | `ServiceBackendsDegraded` | `network` |
| Budget.BlocksDrain | degraded | `PdbBlocksDrain` | `budget_drain` |
| Budget.Overlap | degraded | `PdbOverlap` | `budget` |
| Budget.SelectsNothing | degraded | `PdbSelectsNothing` | `budget` |
| Budget.SyncFailed | degraded | `PdbSyncFailed` | `budget` |
| CPUHigh | degraded | `ContainerCPUUsageHigh` | `resources` |
| CPUThrottled | varies | `ContainerCPUThrottled` | `resources` |
| CRD.NotEstablished | degraded | `CustomResourceDefinitionNotEstablished` | `custom_kinds` |
| CSRFailed | varies | `CertificateSigningRequestFailure` | `custom` |
| CannotRun | failing | `ContainerCannotRun` | `container` |
| Cert.Expired | varies | `TLSCertExpired` | `config` |
| Cert.Expiring | degraded | `TLSCertExpiringSoon` | `config` |
| Certificate.Denied | degraded | `CertificateSigningRequestDenied` | `custom_kinds` |
| Certificate.NotIssued | degraded | `CertificateSigningRequestNotIssued` | `custom_kinds` |
| ClaimFailed | varies | `PersistentVolumeClaimFailure`, `ResourceClaimFailure` | `custom`, `storage` |
| Cluster.VersionSkew | degraded | `ClusterVersionSkew` | `version_skew` |
| Completed | varies | `Completed`, `PodCompleted` | - |
| Condition | varies | `ConditionFailure` | - |
| ConditionFailure | varies | `DaemonSetConditionFailure`, `StatefulSetConditionFailure` | - |
| CrashLoop | failing | `CrashLoopBackOff` | `container` |
| CrashLoop.Liveness | failing | `LivenessKilled` | `container_liveness` |
| CreateError | failing | `CreateContainerError` | `container` |
| CreateError.Config | failing | `CreateContainerConfigError` | `container` |
| Creating | varies | `ContainerCreating` | - |
| DeadlineExceeded | varies | `DeadlineExceeded` | - |
| DeprecatedAPI | degraded | `DeprecatedAPIInUse` | `deprecated_api` |
| Device.PrepareFailed | failing | `FailedPrepareDynamicResources` | `event`, `event_storage` |
| Device.Unallocated | degraded | `ResourceClaimUnallocated` | `custom_kinds` |
| Disk.ContainerGCFailed | degraded | `ContainerGCFailed` | `event`, `event_kubelet` |
| Disk.EvictionThreshold | degraded | `EvictionThresholdMet` | `event`, `event_kubelet` |
| Disk.FreeSpaceFailed | degraded | `FreeDiskSpaceFailed` | `event`, `event_kubelet` |
| Disk.ImageGCFailed | degraded | `ImageGCFailed` | `event`, `event_kubelet` |
| DiskPressure | varies | `DiskPressure` | `node` |
| DisruptionBudget | degraded | `PdbViolation` | `policy` |
| Draining | degraded | `NodeDraining` | `node` |
| EphemeralStorageHigh | varies | `ContainerEphemeralStorageUsageHigh` | `resources` |
| Error | varies | `Error` | `container` |
| Etcd.Large | degraded | `EtcdDatabaseLarge` | `control_plane_storage` |
| Evicted | varies | `Evicted` | `pod` |
| Evicting | degraded | `NodeEvicting` | `kubelet_health` |
| Exit.CommandNotFound | varies | `ContainerExitCommandNotFound` | `container_exit` |
| Exit.Killed | varies | `ContainerExitKilled` | `container_exit` |
| Exit.NotExecutable | varies | `ContainerExitNotExecutable` | `container_exit` |
| Exit.Segfault | varies | `ContainerExitSegfault` | `container_exit` |
| Failed | degraded | `PhaseFailed`, `PodFailed` | `generic`, `pod` |
| Filesystem.Usage | varies | `NodeFilesystemUsageCritical`, `NodeFilesystemUsageHigh` | `usage` |
| FlowControl | varies | `APIPriorityAndFairnessFailure` | `custom` |
| Heartbeat.Stale | degraded | `NodeHeartbeatStale` | `node_health` |
| Hook.PostStart | failing | `PostStartHookError` | `container` |
| Hook.PreStart | failing | `PreStartHookError` | `container` |
| ImageDrift | degraded | `ImageDigestDrift` | `image_drift` |
| ImagePull | failing | `ErrImagePull`, `ImagePullBackOff` | `container` |
| ImagePull.Inspect | failing | `ImageInspectError` | `container` |
| ImagePull.InvalidName | failing | `InvalidImageName` | `container` |
| ImagePull.NeverPull | failing | `ErrImageNeverPull` | `container` |
| ImagePull.Registry | varies | `RegistryUnavailable` | - |
| Ingress.ClassMissing | degraded | `IngressClassMissing` | `ingress` |
| Ingress.TLSSecretMissing | degraded | `IngressTLSSecretMissing` | `ingress` |
| InitError | varies | `InitContainerError` | `container` |
| Initializing | varies | `PodInitializing` | - |
| Inodes.Usage | varies | `NodeInodesUsageCritical`, `NodeInodesUsageHigh` | `usage` |
| InvalidLimitRange | degraded | `LimitRangeInvalid` | `namespace` |
| InvalidPolicy | varies | `AdmissionPolicyInvalid` | `custom` |
| InvalidPolicy.Binding | varies | `AdmissionPolicyBindingInvalid` | `custom` |
| InvalidPolicy.Mutating | varies | `MutatingAdmissionPolicyInvalid` | `custom` |
| InvalidPolicy.Security | degraded | `PodSecurityPolicyInvalid` | `namespace` |
| InvalidSchedule | degraded | `CronJobInvalidSchedule` | `schedule` |
| JobFailed | varies | `JobFailed` | `workload` |
| JobFailed.BackoffLimit | varies | `JobBackoffLimitExceeded` | `workload` |
| JobFailed.Deadline | varies | `JobDeadlineExceeded` | `workload` |
| JobRunningLong | degraded | `JobRunningLong` | `job_runtime` |
| Killed | varies | `Killed` | - |
| Kubelet.Unreachable | degraded | `KubeletUnreachable` | `kubelet_health` |
| Kwatch.NetworkRestricted | degraded | `KwatchNetworkRestricted` | `active_probe` |
| Latency.APIServer | degraded | `APIServerLatency` | `controlplane` |
| Latency.APIServer.Reads | varies | `APIServerReadsSlow` | `control_plane_load` |
| Latency.APIServer.Writes | varies | `APIServerWritesSlow` | `control_plane_load` |
| LeaseStale | degraded | `LeaseStale` | `lease` |
| LoadBalancer.SyncFailed | degraded | `LoadBalancerSyncFailed` | `loadbalancer` |
| LoadBalancerPending | degraded | `LoadBalancerProvisioning` | `network` |
| MemoryHigh | varies | `ContainerMemoryUsageHigh` | `resources` |
| MemoryOvercommitted | degraded | `NodeMemoryOvercommitted` | `node_commitment` |
| MemoryPressure | varies | `MemoryPressure`, `NodeMemoryPressure` | `node` |
| Missing.ConfigMap | varies | `ProjectedConfigMapMissing` | `config` |
| Missing.Secret | varies | `ProjectedSecretMissing` | `config` |
| Missing.ServiceAccount | varies | `ServiceAccountMissing` | `config` |
| Network.CNINotReady | varies | `NodeCNINotReady` | `node_health` |
| Network.IPExhausted | varies | `NodePodIPExhausted` | `node_health` |
| NetworkErrors | varies | `NodeNetworkErrors` | `resources` |
| NetworkPolicy | degraded | `RestrictiveNetworkPolicy` | `network` |
| NetworkUnavailable | failing | `NetworkUnavailable` | `node` |
| NoEndpoints | failing | `ServiceNoEndpoints` | `network`, `service_selector` |
| NotReady | varies | `ContainersNotReady`, `NodeNotReady`, `NotReady` | `node`, `pod` |
| NotReady.Never | varies | `WorkloadNeverReady` | `workload_never_ready` |
| NotReconciling | degraded | `CustomResourceFailure`, `GenerationLagging` | `custom`, `generic` |
| NotScheduling | degraded | `CronJobNotScheduled` | `schedule` |
| OOMKilled | failing | `OOMKILLED`, `OOMKilled` | `container` |
| PIDPressure | varies | `PIDPressure` | `node` |
| PLEGSlow | degraded | `NodePLEGSlow` | `kubelet_health` |
| Pending | degraded | `PhasePending`, `PodPending` | `generic`, `pod` |
| Preempted.Repeatedly | degraded | `PodPreemptedRepeatedly` | `pod_preemption` |
| PressureStall | degraded | `NodePressureStall` | `usage` |
| Probe.Liveness | varies | `LivenessProbeFailed` | `container_probe` |
| Probe.Readiness | varies | `ReadinessProbeFailed` | `container_probe` |
| Probe.Startup | varies | `StartupProbeFailed` | `container_probe` |
| Quota.NearLimit | degraded | `ResourceQuotaNearLimit` | `quota_attach` |
| QuotaExhausted | degraded | `ResourceQuotaExhausted` | `policy` |
| Reference.PriorityClassMissing | varies | `PriorityClassMissing` | `references` |
| Reference.RuntimeClassMissing | varies | `RuntimeClassMissing` | `config`, `references` |
| ReleaseRegression | degraded | `ReleaseRegression` | `release_watch` |
| ReplicaFailure | failing | `DeploymentReplicaFailure`, `ReplicaSetFailure` | `workload` |
| Resize.Deferred | varies | `PodResizeDeferred` | `pod_resize` |
| Resize.Error | varies | `PodResizeError` | `pod_resize` |
| Resize.Infeasible | varies | `PodResizeInfeasible` | `pod_resize` |
| Resolution.Servfail | varies | `CoreDNSServfail` | `controlplane` |
| ResourceCritical | varies | `NodeResourceCritical` | `resources` |
| ResourceHigh | varies | `NodeResourceHigh` | `resources` |
| Restarting | degraded | `HighRestartCount` | `container` |
| Risk | varies | `Risk.` | - |
| Risk.MutableImageTag | varies | `Risk.MutableImageTag` | `risk` |
| Risk.NoMemoryLimit | varies | `Risk.NoMemoryLimit` | `risk` |
| Risk.NoReadinessProbe | varies | `Risk.NoReadinessProbe` | `risk` |
| Risk.Privileged | varies | `Risk.Privileged` | `risk` |
| Risk.SingleNode | varies | `Risk.SingleNode` | `risk` |
| Risk.SingleReplica | varies | `Risk.SingleReplica` | `risk` |
| Rollout.Stuck | degraded | `StatefulSetRolloutStuck` | `rollout` |
| RolloutStuck | failing | `DeploymentProgressingFalse`, `ProgressDeadlineExceeded` | `workload` |
| RuntimeErrors | varies | `NodeRuntimeErrors` | `resources` |
| ScaledToZero | varies | `ScaledToZeroRouted` | `scaled_to_zero` |
| Scaling.Disabled | varies | `ScalingDisabled` | - |
| Scaling.Error | varies | `HPAScalingError` | `hpa` |
| Scaling.InvalidSelector | varies | `HPAInvalidSelector` | `hpa` |
| Scaling.MaxedOut | degraded | `HPAMaxedOut` | `hpa` |
| Scaling.NoMetrics | varies | `FailedComputeMetricsReplicas`, `FailedGetMetrics`, `FailedGetResourceMetric` | `hpa` |
| Scaling.TargetMissing | degraded | `FailedGetScale`, `HPATargetMissing` | `hpa`, `hpa_target` |
| Scaling.UpdateFailed | varies | `FailedUpdateScale` | `hpa` |
| Schedule.Blocked | degraded | `CronJobBlocked` | `schedule_history` |
| Schedule.LastRunFailed | degraded | `CronJobLastRunFailed` | `schedule_last_run` |
| Schedule.Missed | degraded | `CronJobMissedRuns` | `schedule_history` |
| Schedule.NoRecentSuccess | degraded | `CronJobNoRecentSuccess` | `schedule_stale` |
| Schedule.RepeatedFailure | degraded | `CronJobRepeatedFailure` | `schedule`, `schedule_history` |
| SchedulingGated | degraded | `SchedulingGated` | `pod` |
| SnapshotFailed | varies | `VolumeSnapshotFailure` | `custom` |
| StatusUnknown | degraded | `PodStatusUnknown` | `pod` |
| Storage.ObjectsHigh | degraded | `StorageObjectsHigh` | `control_plane_storage` |
| StuckDeleting | degraded | `NamespaceStuckTerminating`, `NodeStuckTerminating`, `PodStuckTerminating`, `StuckDeleting` | `generic`, `namespace`, `node`, `pod` |
| Suspended | varies | `CronJobSuspended`, `JobSuspended` | `schedule` |
| Unavailable | varies | `DaemonSetUnavailable`, `DeploymentAvailableFalse`, `DeploymentUnavailable`, `StsUnavailable` | `workload` |
| Unavailable.APIServer | varies | `APIServerUnavailable` | `controlplane` |
| Unavailable.ControllerManager | varies | `ControllerManagerUnavailable` | `controlplane` |
| Unavailable.CoreDNS | varies | `CoreDNSUnavailable` | `controlplane` |
| Unavailable.Etcd | varies | `EtcdUnavailable` | `controlplane` |
| Unavailable.Scheduler | varies | `SchedulerUnavailable` | `controlplane` |
| Unschedulable | degraded | `FailedScheduling`, `Unschedulable` | `pod` |
| Unused | degraded | `ClaimUnused`, `ServiceUnused` | `network`, `storage` |
| UnusualEvent | varies | `UnusualEvent.` | - |
| Volume.AttachWaiting | varies | `VolumeAttachWaiting` | `event_storage` |
| Volume.DetachFailed | degraded | `VolumeDetachFailure` | `quota_attach` |
| Volume.MapFailed | failing | `FailedMapVolume` | `event`, `event_storage` |
| Volume.ProvisioningFailed | varies | `ProvisioningFailed` | `event_storage` |
| VolumeFailed | failing | `PersistentVolumeFailure` | `storage` |
| VolumeFillingUp | varies | `VolumeFillingUp` | `usage` |
| VolumeFull | varies | `VolumeUsageHigh` | `usage` |
| Webhook.BackendMissing | varies | `WebhookBackendNotFound` | `policy` |
| Webhook.NoEndpoints | varies | `WebhookNoEndpoints` | `policy` |
| Webhook.Rejecting | degraded | `WebhookRejecting` | `webhook_calls` |
| Webhook.Slow | varies | `WebhookSlow` | `webhook_calls` |

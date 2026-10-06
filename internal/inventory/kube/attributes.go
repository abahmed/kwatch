package kube

// Attribute names shared by schemas, detectors and rules. Names are stable:
// they are persisted and referenced by rule definitions.
const (
	AttrPhase      = "phase"
	AttrLabels     = "labels"
	AttrReady      = "ready"
	AttrReadySince = "ready.transition"
	AttrReason     = "reason"
	AttrMessage    = "message"
	AttrDeleting   = "deleting"
	// AttrDeletionTime is when the pod's deletion was requested; set only
	// while it is being deleted. AttrTerminationGrace is the seconds its
	// containers are given to stop (spec.terminationGracePeriodSeconds).
	AttrDeletionTime     = "deletion.timestamp"
	AttrTerminationGrace = "termination.grace.seconds"
	AttrQoS              = "qos"
	AttrScheduled        = "scheduled"
	AttrScheduledReason  = "scheduled.reason"
	AttrStartTime        = "start.time"
	// AttrCompletionTime is when a Job finished all its pods, and
	// AttrActiveDeadline is its spec.activeDeadlineSeconds.
	AttrCompletionTime = "completion.time"
	AttrActiveDeadline = "active.deadline.seconds"
	// AttrCreated is the pod creation time, for pending and ready
	// baselines.
	AttrCreated = "created"

	AttrState        = "state"
	AttrStateReason  = "state.reason"
	AttrExitCode     = "exit.code"
	AttrRestarts     = "restarts"
	AttrLastReason   = "last.reason"
	AttrLastExitCode = "last.exit.code"
	AttrLastFinished = "last.finished"
	AttrLastMessage  = "last.message"
	// AttrLastStarted is when the last terminated run started, so the
	// length of that run is last.finished minus last.started.
	AttrLastStarted = "last.started"
	// AttrContainerPorts lists the container's declared port numbers.
	AttrContainerPorts = "container.ports"
	// AttrProbes lists the probes the container declares, in startup,
	// readiness, liveness order: "readiness,liveness". Absent when none.
	AttrProbes = "probes"
	// AttrPrivileged is true for a container that runs privileged.
	AttrPrivileged = "privileged"
	// AttrPodIP is the pod's address, for the metrics the prober reads
	// from the cluster DNS pods.
	AttrPodIP = "pod.ip"
	// Lease attributes, written by the prober's Lease scan.
	AttrLeaseHolder   = "lease.holder"
	AttrLeaseRenewed  = "lease.renewed"
	AttrLeaseDuration = "lease.duration.seconds"
	// AttrImageID is the image the container actually runs, by digest,
	// as the kubelet reports it; two pods with the same image tag and
	// different IDs run different builds.
	AttrImageID = "image.id"
	// AttrProbePorts lists the ports the container's probes connect to,
	// named ports resolved against the container's own port names.
	AttrProbePorts        = "probe.ports"
	AttrStartedAt         = "started.at"
	AttrImage             = "image"
	AttrInit              = "init"
	AttrSidecar           = "sidecar"
	AttrMemoryLimit       = "memory.limit"
	AttrMemoryReq         = "memory.request"
	AttrCPULimit          = "cpu.limit"
	AttrCPUReq            = "cpu.request"
	AttrEphemeralLimit    = "ephemeral.limit"
	AttrCPUAllocatable    = "cpu.allocatable"
	AttrMemoryAllocatable = "memory.allocatable"
	AttrProbeBudget       = "probe.budget.seconds"

	AttrUnschedulable = "unschedulable"
	AttrTaints        = "taints"
	AttrKubelet       = "kubelet.version"
	AttrServerVersion = "server.version"
	AttrServerMinor   = "server.version.minor"
	AttrRuntime       = "runtime.version"
	AttrKernel        = "kernel.version"
	AttrOS            = "os"
	AttrInstanceType  = "instance.type"

	AttrReplicas         = "replicas"
	AttrReadyReplicas    = "replicas.ready"
	AttrAvailable        = "replicas.available"
	AttrUpdatedReplicas  = "replicas.updated"
	AttrUnavailable      = "replicas.unavailable"
	AttrGeneration       = "generation"
	AttrObservedGen      = "generation.observed"
	AttrTemplateHash     = "template.hash"
	AttrRevision         = "revision"
	AttrProgressDeadline = "progress.deadline.exceeded"
	AttrConditionPrefix  = "condition."
	AttrConditionReason  = ".reason"
	AttrConditionMessage = ".message"
	AttrConditionSince   = ".since"
	AttrSuspended        = "suspended"
	AttrLastSchedule     = "last.schedule"
	AttrLastSuccess      = "last.success"
	AttrNextRun          = "next.run"
	AttrScheduleProblem  = "schedule.problem"
	AttrActive           = "active"
	AttrSucceeded        = "succeeded"
	AttrFailed           = "failed"
	AttrBackoffLimit     = "backoff.limit"
	AttrCurrentReplicas  = "replicas.current"
	AttrDesiredReplicas  = "replicas.desired"
	AttrMinReplicas      = "replicas.min"
	AttrMaxReplicas      = "replicas.max"
)

// AttrTemplateLabels are the labels of the pods a workload creates, in
// the format of AttrLabels. They stay known when the workload has no
// pods, so a Service can still be matched to it.
const AttrTemplateLabels = "template.labels"

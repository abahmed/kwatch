package detection

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// podAndContainer are the kinds the kubelet writes its container events
// about: the container for an event with a container field path, the pod
// for the rest.
var podAndContainer = []inventory.Kind{kube.KindPod, kube.KindContainer}

// eventOnlyReasons are Kubernetes event reasons and container states that
// have no reason constant (and so no mode) of their own.
//
// A Warning event that is a failure on its own and is not already
// detected from object state has an Event severity. Reasons that state
// already shows (BackOff, Unhealthy, FailedScheduling, Evicted, HPA and
// Job conditions) stay evidence only, so nothing is reported twice; they
// are marked shown. Webhook failures surface as FailedCreate, there is
// no FailedCallingWebhook event; cluster-autoscaler's NotTriggerScaleUp
// is a Normal event, which is never ingested.
var eventOnlyReasons = []ReasonInfo{
	eventReason("FailedMount", Critical).
		label("can't mount a volume"),
	eventReason("FailedAttachVolume", Critical).
		label("can't attach a volume"),
	eventReason("FailedCreatePodSandBox", Critical),
	eventReason("FailedCreate", Critical).
		label("can't create its pods"),
	eventReason("FailedBinding", Warning),
	eventReason("VolumeResizeFailed", Warning),
	eventReason("FailedDaemonPod", Warning),
	eventReason("NetworkNotReady", Critical),
	eventReason("FailedKillPod", Warning),
	eventReason("FailedPreStopHook", Warning),
	eventReason("FailedPostStartHook", Critical),
	// Labelled only: the load balancer controller's own failures.
	{Reason: "FailedDeployModel",
		Label: "failed to deploy its load balancer"},
	{Reason: "FailedBuildModel",
		Label: "failed to build its load balancer"},
	// A container waiting state that is a failure.
	{Reason: "RunContainerError", Waiting: Critical},
	{Reason: "PreCreateHookError", Waiting: Critical},
	// Passed through from the CRI: the image cannot be used as it is.
	{Reason: "SignatureValidationFailed", Waiting: Critical},
	{Reason: "RROUnsupported", Waiting: Critical},
	{Reason: "ImageVolumeMountFailed", Waiting: Critical},
	// The kubelet records these against the container (field path
	// spec.containers{name}), so a container shows them as well as a pod.
	stateReason("BackOff", podAndContainer...),
	stateReason("Unhealthy", podAndContainer...),
	stateReason("Failed", podAndContainer...),
	stateReason("Killing", podAndContainer...),
	stateReason("Preempting", kube.KindPod),
	stateReason("Preempted", kube.KindPod),
	stateReason("InspectFailed", podAndContainer...),
	stateReason("NodeNotSchedulable", kube.KindNode),
	stateReason("BackoffLimitExceeded", kube.KindJob),
	stateReason("TooManyMissedTimes", kube.KindCronJob),
	// No detector reads the endpoint controller's failures from the
	// Service, so they are not shown by state: a repeat is reported.
	{Reason: "FailedToUpdateEndpointSlices"},
	{Reason: "FailedToUpdateEndpoint"},
	stateReason("OOMKilling", kube.KindNode),
	// The upstream corpus lists reasons, not the kinds they are about, so
	// no kind is claimed to show FailedValidation: a repeat is reported.
	{Reason: "FailedValidation"},
	stateReason("NoPods", kube.KindPDB),
	stateReason("CalculateExpectedPodCountFailed", kube.KindPDB),
}

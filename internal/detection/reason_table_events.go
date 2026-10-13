package detection

// eventOnlyReasons are Kubernetes event reasons and container states that
// have no reason constant (and so no mode) of their own.
//
// A Warning event that is a failure on its own and is not already
// detected from object state has an Event severity. Reasons that state
// already shows (BackOff, Unhealthy, FailedScheduling, Evicted, HPA and
// Job conditions) stay evidence only, so nothing is reported twice; they
// are marked shown. Webhook failures surface as FailedCreate;
// cluster-autoscaler's NotTriggerScaleUp is a Normal event, which is
// never ingested.
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
	{Reason: "FailedDeployModel", Label: "can't deploy its load balancer"},
	{Reason: "FailedBuildModel", Label: "can't build its load balancer"},
	// A container waiting state that is a failure.
	{Reason: "RunContainerError", Waiting: Critical},
	{Reason: "PreCreateHookError", Waiting: Critical},
	stateReason("BackOff"), stateReason("Unhealthy"),
	stateReason("Failed"), stateReason("Killing"),
	stateReason("Preempting"), stateReason("Preempted"),
	stateReason("InspectFailed"), stateReason("NodeNotSchedulable"),
	stateReason("BackoffLimitExceeded"), stateReason("TooManyMissedTimes"),
	stateReason("FailedToUpdateEndpointSlices"),
	stateReason("FailedToUpdateEndpoint"),
	stateReason("OOMKilling"), stateReason("FailedCallingWebhook"),
	stateReason("FailedValidation"), stateReason("NoPods"),
	stateReason("CalculateExpectedPodCountFailed"),
}

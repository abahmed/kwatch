package detector

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// ContainerDetector detects issues in container status
type ContainerDetector struct{}

func NewContainerDetector() *ContainerDetector {
	return &ContainerDetector{}
}

func (d *ContainerDetector) Name() string {
	return "ContainerDetector"
}

func (d *ContainerDetector) Detect(input *Input) bool {
	if input.Container == nil {
		return false
	}

	container := input.Container
	input.RestartCount = container.RestartCount

	// Check waiting state
	if container.State.Waiting != nil {
		input.HasIssue = true
		input.IssueType = "container"
		input.Reason = container.State.Waiting.Reason
		input.Message = container.State.Waiting.Message
		input.Status = "waiting"
		return true
	}

	// Check terminated state
	if container.State.Terminated != nil {
		input.HasIssue = true
		input.IssueType = "container"
		input.Reason = container.State.Terminated.Reason
		input.Message = container.State.Terminated.Message
		input.ExitCode = container.State.Terminated.ExitCode
		input.Status = "terminated"
		input.LastTerminatedOn = container.State.Terminated.StartedAt.Time
		return true
	}

	// Check if restarting (has restarts but running)
	if container.RestartCount > 0 && container.State.Running != nil {
		input.HasIssue = true
		input.IssueType = "container"
		input.Reason = "Restarting"
		input.Message = "Container has restarts"
		input.Status = "running"
		return true
	}

	return false
}

// ContainerState holds extracted container state info
type ContainerState struct {
	Reason           string
	Message          string
	ExitCode         int32
	Status           string
	LastTerminatedOn time.Time
}

// ExtractContainerState extracts state from container status
func ExtractContainerState(container *corev1.ContainerStatus) ContainerState {
	state := ContainerState{}

	if container.State.Waiting != nil {
		state.Reason = container.State.Waiting.Reason
		state.Message = container.State.Waiting.Message
		state.Status = "waiting"
	} else if container.State.Terminated != nil {
		state.Reason = container.State.Terminated.Reason
		state.Message = container.State.Terminated.Message
		state.ExitCode = container.State.Terminated.ExitCode
		state.Status = "terminated"
		state.LastTerminatedOn = container.State.Terminated.StartedAt.Time
	} else if container.State.Running != nil {
		state.Status = "running"
	}

	// Check last termination state for CrashLoopBackOff
	if container.LastTerminationState.Terminated != nil {
		if state.Reason == "" || state.Reason == "CrashLoopBackOff" {
			state.Reason = container.LastTerminationState.Terminated.Reason
			state.Message = container.LastTerminationState.Terminated.Message
			state.ExitCode = container.LastTerminationState.Terminated.ExitCode
			state.LastTerminatedOn = container.LastTerminationState.Terminated.StartedAt.Time
		}
	}

	return state
}

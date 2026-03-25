package common

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

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

// IsGracefulExit checks if container exited gracefully
func IsGracefulExit(exitCode int32) bool {
	return exitCode == 0 || exitCode == 143
}

// IsClearFailure checks if reason indicates a clear failure
func IsClearFailure(reason string) bool {
	clearFailures := []string{
		"CrashLoopBackOff",
		"OOMKilled",
		"ImagePullBackOff",
		"ErrImagePull",
		"CreateContainerError",
	}
	for _, f := range clearFailures {
		if reason == f {
			return true
		}
	}
	return false
}

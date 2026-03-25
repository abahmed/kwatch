package handler

import (
	"context"

	"github.com/abahmed/kwatch/internal/detector"
	"github.com/abahmed/kwatch/internal/util"
)

// ContainerHandler enriches container events with state info
type ContainerHandler struct {
	client interface {
		CoreV1() interface {
			Pods(string) interface {
				GetLogs(string, interface{}) *restRequest
			}
		}
	}
}

type restRequest struct {
	ctx context.Context
}

func NewContainerHandler() *ContainerHandler {
	return &ContainerHandler{}
}

func (h *ContainerHandler) Name() string {
	return "ContainerHandler"
}

func (h *ContainerHandler) Handle(input *detector.Input) error {
	// Container state enrichment is done in the detector
	// This handler can be used for additional container-specific enrichment

	if input.Container == nil || input.Pod == nil {
		return nil
	}

	// The container state info is already extracted by ContainerDetector
	// Additional enrichment like logs can be added here in the future

	return nil
}

// EnrichWithState enriches input with container state information
func EnrichWithState(input *detector.Input) {
	if input.Container == nil {
		return
	}

	// Extract and set container state info
	state := detector.ExtractContainerState(input.Container)
	if input.Reason == "" {
		input.Reason = state.Reason
	}
	if input.Message == "" {
		input.Message = state.Message
	}
	if input.ExitCode == 0 {
		input.ExitCode = state.ExitCode
	}
	if input.Status == "" {
		input.Status = state.Status
	}
	if input.LastTerminatedOn.IsZero() && !state.LastTerminatedOn.IsZero() {
		input.LastTerminatedOn = state.LastTerminatedOn
	}
}

// EnrichWithLogs enriches input with container logs
func EnrichWithLogs(input *detector.Input) error {
	if input.Container == nil || input.Pod == nil {
		return nil
	}

	logs := util.GetPodContainerLogs(
		input.Client,
		input.Pod.Name,
		input.Container.Name,
		input.Pod.Namespace,
		false,
		0,
	)

	input.Logs = logs
	return nil
}

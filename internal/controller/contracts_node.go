package controller

import (
	"github.com/abahmed/kwatch/internal/model"
	nodemonitor "github.com/abahmed/kwatch/internal/monitor/node"
)

// NodeProcessor handles Node observations and periodic resource checks.
type NodeProcessor interface {
	ProcessNode(string, bool) error
	ProcessNodeResourceOvercommit(string, string, string, model.Severity)
}

type NodeConfig interface {
	ConfigureSources(nodemonitor.Sources) error
}

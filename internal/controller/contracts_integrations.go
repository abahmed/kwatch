package controller

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/controlplane"
	securitymonitor "github.com/abahmed/kwatch/internal/monitor/security"
)

type ControlPlaneProcessor interface {
	ProcessControlPlanePod(*corev1.Pod) error
	SweepControlPlane()
}

type ControlPlaneConfig interface {
	ConfigureSources(controlplane.Sources) error
}

type TLSProcessor interface {
	SweepTLSSecrets()
}

type TLSConfig interface {
	ConfigureSources(securitymonitor.TLSSources) error
}

type EventProcessor interface {
	ProcessClusterAutoscalerEvent(*corev1.Event)
	ProcessWarningEvent(*corev1.Event)
}

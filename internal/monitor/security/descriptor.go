package security

import (
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/monitor"
)

// Descriptor returns the stable metadata for the security monitor family.
func Descriptor() monitor.Descriptor {
	return monitor.Descriptor{
		Name:        "security-monitor",
		Description: "Detect security and admission failures",
		Feature:     feature.SecurityDetection,
		Resources: []string{
			"mutatingwebhookconfiguration",
			"validatingwebhookconfiguration",
			"service",
			"endpointslice",
			"secret",
		},
		Documentation: "/docs/general-configuration",
	}
}

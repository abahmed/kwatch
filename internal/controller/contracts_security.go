package controller

import securitymonitor "github.com/abahmed/kwatch/internal/monitor/security"

type SecurityProcessor interface {
	ProcessMutatingWebhookConfiguration(string, bool) error
	ProcessValidatingWebhookConfiguration(string, bool) error
}

type SecurityConfig interface {
	ConfigureSources(securitymonitor.Sources) error
}

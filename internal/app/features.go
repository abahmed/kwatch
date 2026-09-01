package app

import (
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/feature"
)

// buildFeaturePlan is the only place where configuration is translated into
// product capabilities. Monitors receive the resulting decision, so tier
// checks do not spread through detection and correlation code.
func buildFeaturePlan(cfg *config.Config, now time.Time) (feature.Plan, error) {
	requested := requestedFeatures(cfg)
	disabled := make([]feature.ID, 0, len(cfg.Features.Disabled))
	for _, id := range cfg.Features.Disabled {
		disabled = append(disabled, feature.ID(id))
	}
	return feature.Build(feature.CommunityPolicy(), requested, feature.Overrides{Disabled: disabled}, now)
}

// applyFeaturePlan projects fine-grained decisions back onto legacy monitor
// switches. This keeps existing constructors stable while ensuring a disabled
// capability cannot still be started by an older wiring path.
func applyFeaturePlan(cfg *config.Config, plan feature.Plan) {
	gate(&cfg.ScheduleMonitor.Enabled, plan, feature.PodScheduling)
	gate(&cfg.PendingPodMonitor.Enabled, plan, feature.PodPending)
	gate(&cfg.OomMonitor.Enabled, plan, feature.PodOOM)
	gate(&cfg.NotReadyMonitor.Enabled, plan, feature.PodReadiness)
	gateInt(&cfg.ContainerRestartThreshold, plan, feature.PodRestarts)
	gate(&cfg.RolloutMonitor.Enabled, plan, feature.DeploymentRollout)
	gate(&cfg.StatefulSetMonitor.Enabled, plan, feature.StatefulSetRollout)
	gate(&cfg.DaemonSetMonitor.Enabled, plan, feature.DaemonSetRollout)
	gate(&cfg.JobMonitor.Enabled, plan, feature.JobFailures)
	gate(&cfg.CronJobMonitor.Enabled, plan, feature.CronJobFailures)
	gate(&cfg.PdbMonitor.Enabled, plan, feature.PDBViolations)
	gate(&cfg.HpaMonitor.Enabled, plan, feature.HPADiagnostics)
	gate(&cfg.NodeMonitor.Enabled, plan, feature.NodeConditions)
	gate(&cfg.NodeResourceMonitor.Enabled, plan, feature.NodeResources)
	gate(&cfg.PvcMonitor.Enabled, plan, feature.PVCUsage)
	gate(&cfg.ServiceMonitor.Enabled, plan, feature.ServiceEndpoints)
	gate(&cfg.IngressMonitor.Enabled, plan, feature.IngressBackends)
	gate(&cfg.NetworkPolicyMonitor.Enabled, plan, feature.NetworkPolicies)
	gate(&cfg.AdmissionWebhookMonitor.Enabled, plan, feature.AdmissionWebhooks)
	gate(&cfg.TlsMonitor.Enabled, plan, feature.TLSSignals)
	gate(&cfg.ClusterResourceMonitor.Enabled, plan, feature.ClusterResources)
	gate(&cfg.AdaptiveThresholds, plan, feature.AdaptiveBaseline)
	gateInt(&cfg.SmartGrouping.WindowSeconds, plan, feature.SmartGrouping)
	gateInt(&cfg.Correlation.CooldownMinutes, plan, feature.Cooldown)
	gate(&cfg.Correlation.Escalation.Enabled, plan, feature.Escalation)
	clearIfDisabled(plan, feature.AuditLog, func() { cfg.AuditLog.Enabled = false })
	clearIfDisabled(plan, feature.CustomTemplates, func() { cfg.Templates = nil })
	clearIfDisabled(plan, feature.Runbooks, func() { cfg.Runbooks = nil })
}

func gate(value *bool, plan feature.Plan, id feature.ID) {
	*value = *value && plan.Enabled(id)
}

func gateInt(value *int, plan feature.Plan, id feature.ID) {
	if !plan.Enabled(id) {
		*value = 0
	}
}

func clearIfDisabled(plan feature.Plan, id feature.ID, clear func()) {
	if !plan.Enabled(id) {
		clear()
	}
}

func requestedFeatures(cfg *config.Config) map[feature.ID]bool {
	requested := map[feature.ID]bool{
		feature.PodDetection:           true,
		feature.PodScheduling:          cfg.ScheduleMonitor.Enabled,
		feature.PodPending:             cfg.PendingPodMonitor.Enabled,
		feature.PodOOM:                 cfg.OomMonitor.Enabled,
		feature.PodReadiness:           cfg.NotReadyMonitor.Enabled,
		feature.PodRestarts:            cfg.ContainerRestartThreshold > 0,
		feature.PodLifecycle:           true,
		feature.WorkloadDetection:      true,
		feature.DeploymentRollout:      cfg.RolloutMonitor.Enabled,
		feature.StatefulSetRollout:     cfg.StatefulSetMonitor.Enabled,
		feature.DaemonSetRollout:       cfg.DaemonSetMonitor.Enabled,
		feature.JobFailures:            cfg.JobMonitor.Enabled,
		feature.CronJobFailures:        cfg.CronJobMonitor.Enabled,
		feature.PDBViolations:          cfg.PdbMonitor.Enabled,
		feature.HPADiagnostics:         cfg.HpaMonitor.Enabled,
		feature.NodeDetection:          true,
		feature.NodeConditions:         cfg.NodeMonitor.Enabled,
		feature.NodeResources:          cfg.NodeResourceMonitor.Enabled,
		feature.StorageDetection:       cfg.PvcMonitor.Enabled || cfg.ClusterResourceMonitor.Enabled,
		feature.PVCUsage:               cfg.PvcMonitor.Enabled,
		feature.NetworkDetection:       cfg.ServiceMonitor.Enabled || cfg.IngressMonitor.Enabled || cfg.NetworkPolicyMonitor.Enabled,
		feature.ServiceEndpoints:       cfg.ServiceMonitor.Enabled,
		feature.IngressBackends:        cfg.IngressMonitor.Enabled,
		feature.NetworkPolicies:        cfg.NetworkPolicyMonitor.Enabled,
		feature.SecurityDetection:      cfg.AdmissionWebhookMonitor.Enabled || cfg.TlsMonitor.Enabled || cfg.AuditLog.Enabled,
		feature.AdmissionWebhooks:      cfg.AdmissionWebhookMonitor.Enabled,
		feature.ClusterResources:       cfg.ClusterResourceMonitor.Enabled,
		feature.TLSSignals:             cfg.TlsMonitor.Enabled,
		feature.DirectDiagnosis:        true,
		feature.DependencyGraph:        true,
		feature.ImpactAnalysis:         true,
		feature.ChangeDiff:             true,
		feature.IncidentTimeline:       true,
		feature.RCAConfidence:          true,
		feature.RCAFeedback:            true,
		feature.Fingerprinting:         true,
		feature.ConfirmationWindow:     true,
		feature.Cooldown:               cfg.Correlation.CooldownMinutes > 0,
		feature.SmartGrouping:          cfg.SmartGrouping.WindowSeconds > 0,
		feature.MassFailureSuppression: true,
		feature.CascadeSuppression:     true,
		feature.SeverityByBlastRadius:  true,
		feature.IncidentPersistence:    true,
		feature.BaselinePersistence:    true,
		feature.ChangePersistence:      true,
		feature.KubeletTelemetry:       cfg.KubeletTelemetryMonitor.Enabled,
		feature.CPUUsage:               cfg.KubeletTelemetryMonitor.Enabled,
		feature.CPUThrottling:          cfg.KubeletTelemetryMonitor.Enabled,
		feature.MemoryUsage:            cfg.KubeletTelemetryMonitor.Enabled,
		feature.StorageUsage:           cfg.KubeletTelemetryMonitor.Enabled,
		feature.PressureSignals:        cfg.KubeletTelemetryMonitor.Enabled,
		feature.NetworkErrors:          cfg.KubeletTelemetryMonitor.Enabled,
		feature.RuntimeErrors:          cfg.KubeletTelemetryMonitor.Enabled,
		feature.MetricsAPI:             cfg.RuntimeMetricsMonitor.Enabled,
		feature.AdaptiveBaseline:       cfg.AdaptiveThresholds,
		feature.HTTPProbes:             cfg.ActiveProbeMonitor.Enabled && len(cfg.ActiveProbeMonitor.HTTP) > 0,
		feature.TCPProbes:              cfg.ActiveProbeMonitor.Enabled && len(cfg.ActiveProbeMonitor.TCP) > 0,
		feature.DNSProbes:              cfg.ActiveProbeMonitor.Enabled && len(cfg.ActiveProbeMonitor.DNS) > 0,
		feature.AutomaticProbes:        cfg.ActiveProbeMonitor.Enabled && cfg.ActiveProbeMonitor.AutoServices,
		feature.ProbeLatency:           cfg.ActiveProbeMonitor.Enabled,
		// HTTP custom headers are a catalog capability reserved for the probe
		// configuration seam; the current config has no header field, so it is
		// never reported as active by accident.
		feature.ProbeHeaders:            false,
		feature.ControlPlanePods:        cfg.ControlPlaneMonitor.Enabled,
		feature.APIServerHealth:         cfg.ControlPlaneMonitor.Enabled,
		feature.APIServerLatency:        cfg.ControlPlaneMonitor.Enabled && cfg.ControlPlaneMonitor.APIServerLatencyWarningMs > 0,
		feature.SchedulerHealth:         cfg.ControlPlaneMonitor.Enabled,
		feature.ControllerManagerHealth: cfg.ControlPlaneMonitor.Enabled,
		feature.EtcdHealth:              cfg.ControlPlaneMonitor.Enabled,
		feature.InformerHealth:          true,
		feature.GenericStatus:           cfg.ClusterResourceMonitor.Enabled,
		feature.CRDDiscovery:            cfg.CrdConfig.Enabled,
		feature.EventFailureSecurity:    true,
		feature.RBACAudit:               true,
		feature.AdmissionFailures:       cfg.AdmissionWebhookMonitor.Enabled,
		feature.TLSMonitoring:           cfg.TlsMonitor.Enabled,
		feature.AuditLog:                cfg.AuditLog.Enabled,
		feature.SingleDestination:       len(cfg.Alert) > 0,
		feature.MultiDestination:        len(cfg.Alert) > 1,
		// Advanced routing is reserved in the catalog; the current delivery
		// configuration intentionally exposes only basic single/multi
		// destination routing, so do not report a capability that is not yet
		// implemented as a separate runtime behavior.
		feature.AdvancedRouting: false,
		feature.Escalation:      cfg.Correlation.Escalation.Enabled,
		feature.CustomTemplates: len(cfg.Templates) > 0,
		feature.Runbooks:        len(cfg.Runbooks) > 0,
	}
	return requested
}

func activeProbesEnabled(plan feature.Plan) bool {
	return plan.Enabled(feature.HTTPProbes) ||
		plan.Enabled(feature.TCPProbes) ||
		plan.Enabled(feature.DNSProbes) ||
		plan.Enabled(feature.AutomaticProbes)
}

func controlPlaneFeaturesEnabled(plan feature.Plan) bool {
	return plan.Enabled(feature.APIServerHealth) ||
		plan.Enabled(feature.SchedulerHealth) ||
		plan.Enabled(feature.ControllerManagerHealth) ||
		plan.Enabled(feature.EtcdHealth)
}

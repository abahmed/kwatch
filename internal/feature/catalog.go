package feature

import "fmt"

// Lifecycle documents when a capability is relevant to the product. It is
// metadata for the generated catalog consumed by kwatch.sh.
type Lifecycle string

const (
	StartupOnly Lifecycle = "startup"
	Runtime     Lifecycle = "runtime"
)

// Definition is the source of truth for a capability in the generated
// catalog. Dependencies are explicit; there is no implicit inheritance.
type Definition struct {
	ID           ID
	Description  string
	Lifecycle    Lifecycle
	Dependencies []ID
}

var definitions = []Definition{
	{
		PodDetection, "Detect pod and container failures",
		Runtime, nil,
	},
	{
		PodScheduling, "Detect pod scheduling failures",
		Runtime, []ID{PodDetection},
	},
	{
		PodOOM, "Detect out-of-memory kills and repeated OOMs",
		Runtime, []ID{PodDetection},
	},
	{
		PodReadiness, "Detect sustained pod readiness failures",
		Runtime, []ID{PodDetection},
	},
	{
		PodRestarts, "Detect crash loops and excessive restarts",
		Runtime, []ID{PodDetection},
	},
	{
		WorkloadDetection, "Detect workload rollout and execution failures",
		Runtime, nil,
	},
	{
		WorkloadRollouts, "Detect stuck workload rollouts",
		Runtime, []ID{WorkloadDetection},
	},
	{
		JobFailures, "Detect failed Jobs and failed or missed CronJobs",
		Runtime, []ID{WorkloadDetection},
	},
	{
		DisruptionBudgets, "Detect PodDisruptionBudgets that block disruption",
		Runtime, []ID{WorkloadDetection},
	},
	{
		Autoscaling, "Diagnose HorizontalPodAutoscaler failures",
		Runtime, []ID{WorkloadDetection},
	},
	{
		NodeDetection, "Detect node readiness, pressure and draining",
		Runtime, nil,
	},
	{
		NodeUsage, "Detect node resource pressure from kubelet stats",
		Runtime, []ID{NodeDetection},
	},
	{
		StorageDetection, "Detect claim, volume and attachment failures",
		Runtime, nil,
	},
	{
		VolumeUsage, "Predict volumes filling up",
		Runtime, []ID{StorageDetection},
	},
	{
		NetworkDetection, "Detect Service, Ingress and NetworkPolicy failures",
		Runtime, nil,
	},
	{
		AdmissionDetection, "Detect admission webhook and policy failures",
		Runtime, nil,
	},
	{
		CertificateDetection, "Detect expiring and invalid TLS certificates",
		Runtime, nil,
	},
	{
		QuotaDetection, "Detect exhausted ResourceQuotas and LimitRange rejections",
		Runtime, nil,
	},
	{
		CustomResources, "Detect failing conditions on custom and built-in resources",
		Runtime, nil,
	},
	{
		ControlPlane, "Check API server, etcd, DNS, scheduler and controller-manager",
		Runtime, nil,
	},
	{
		ActiveProbes, "Run configured HTTP, TCP and DNS checks",
		Runtime, nil,
	},
	{
		ConfigurationRisks, "Report configuration risks (no probe, no " +
			"limit, mutable tag, single replica) in the digest",
		Runtime, []ID{WorkloadDetection},
	},
	{
		UnusualEvents, "Report repeated Warning events kwatch has no " +
			"detector for, quoting the event",
		Runtime, nil,
	},
	{
		RootCause, "Explain each incident by its most likely root cause",
		Runtime, nil,
	},
	{
		SharedFactors, "Suspect what workloads failing together share " +
			"when nothing else explains them",
		Runtime, []ID{RootCause},
	},
	{
		ChangeCorrelation, "Relate incidents to recent changes and who made them",
		Runtime, []ID{RootCause},
	},
	{
		Impact, "Show which workloads and services an incident affects",
		Runtime, []ID{RootCause},
	},
	{
		Investigation, "Attach a redacted log excerpt to announcements",
		Runtime, nil,
	},
	{
		NoiseControl, "Settle, merge symptoms and send only material changes",
		Runtime, []ID{RootCause},
	},
	{
		Flapping, "Recognise flapping and routine recurring incidents",
		Runtime, []ID{NoiseControl},
	},
	{
		StartupSummary, "Summarise pre-existing incidents once at cold start",
		StartupOnly, nil,
	},
	{
		DiskState, "Keep incidents and history on disk across restarts",
		StartupOnly, nil,
	},
	{
		DowntimeChanges, "Report changes made while kwatch was down",
		StartupOnly, []ID{DiskState},
	},
	{
		Scope, "Filter by namespace, reason, selector and silences",
		Runtime, nil,
	},
	{
		SeverityOverrides, "Override severity by reason and owner kind",
		Runtime, nil,
	},
	{
		Maintenance, "Hold incidents for objects under maintenance",
		Runtime, nil,
	},
	{
		Runbooks, "Attach reason-aware runbook links",
		Runtime, nil,
	},
	{
		ProviderRouting, "Route incidents to providers by scope",
		Runtime, nil,
	},
	{
		NativeThreads, "Update one message thread per incident where supported",
		Runtime, nil,
	},
	{
		CustomTemplates, "Render operator-selected text templates",
		Runtime, nil,
	},
	{
		AuditLog, "Write a JSON line for every incident decision",
		Runtime, nil,
	},
	{
		RBACAudit, "Report missing permissions",
		Runtime, nil,
	},
}

// Catalog returns a copy so callers cannot mutate the product registry.
func Catalog() []Definition {
	result := make([]Definition, len(definitions))
	copy(result, definitions)
	for i := range result {
		result[i].Dependencies = append([]ID(nil), result[i].Dependencies...)
	}
	return result
}

// Lookup returns a definition and whether the ID is known.
func Lookup(id ID) (Definition, bool) {
	for _, definition := range definitions {
		if definition.ID == id {
			return definition, true
		}
	}
	return Definition{}, false
}

// ValidateCatalog rejects duplicate IDs, missing dependencies, and dependency
// cycles.
func ValidateCatalog() error {
	known := make(map[ID]Definition, len(definitions))
	for _, definition := range definitions {
		if definition.ID == "" {
			return fmt.Errorf("feature catalog contains an empty id")
		}
		if _, exists := known[definition.ID]; exists {
			return fmt.Errorf("feature catalog contains duplicate id %q", definition.ID)
		}
		known[definition.ID] = definition
	}
	state := make(map[ID]uint8, len(known))
	var visit func(ID) error
	visit = func(id ID) error {
		switch state[id] {
		case 1:
			return fmt.Errorf("feature catalog dependency cycle at %q", id)
		case 2:
			return nil
		}
		state[id] = 1
		for _, dependency := range known[id].Dependencies {
			if _, exists := known[dependency]; !exists {
				return fmt.Errorf(
					"feature %q depends on unknown feature %q", id, dependency)
			}
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[id] = 2
		return nil
	}
	for id := range known {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

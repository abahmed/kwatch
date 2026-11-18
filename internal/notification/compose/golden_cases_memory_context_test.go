package compose

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

// memoryContextCases are kills and evictions with the memory context
// kwatch holds, and a probe that fails at the CPU limit.
func memoryContextCases() []goldenCase {
	return []goldenCase{
		{"oom_own_limit_sizing", writeOOMOwnLimitSizing},
		{"oom_node_with_request", writeOOMNodeWithRequest},
		{"eviction_memory_pressure", writeEvictionMemory},
		{"probe_at_cpu_limit", writeProbeAtCPULimit},
	}
}

func singleMember(
	root inventory.EntityID, f detection.Finding,
) incident.Incident {
	return incident.Incident{
		ID: "inc-ctx", Root: root, Tier: incident.Notify,
		State: incident.Open, Opened: at(0, 0), Revision: 1,
		Members: members(f),
	}
}

func memEv(label, value string) detection.Evidence {
	return detection.Evidence{Label: label, Value: value}
}

func writeOOMOwnLimitSizing() notification.Message {
	f := detection.Finding{Entity: paymentsCt, Reason: reasons.OOMKilled,
		Mode: detection.ModeOOMKilledLimit, Severity: detection.Critical,
		Since: at(0, 0), Summary: "Container was OOM-killed",
		Evidence: []detection.Evidence{
			memEv(detection.EvidenceMemoryLimit, "512Mi"),
			memEv(detection.EvidenceMemoryPeak, "510Mi"),
			memEv(detection.EvidenceMemoryRequest, "128Mi"),
			memEv(detection.EvidenceOOMKind, "its own memory limit"),
			memEv(detection.EvidenceMemoryWindow, "5h"),
			memEv(detection.EvidenceMemoryFlat, "near 510Mi for 4h"),
			memEv(detection.EvidenceMemoryNearLimit, "4"),
		}}
	return Writer{}.Write(announce(singleMember(payments, f)), at(3, 0))
}

func writeOOMNodeWithRequest() notification.Message {
	f := detection.Finding{Entity: paymentsCt, Reason: reasons.OOMKilled,
		Mode: detection.ModeOOMKilledNode, Severity: detection.Critical,
		Since: at(0, 0), Summary: "Container was OOM-killed by the node " +
			"and not by its own limit",
		Evidence: []detection.Evidence{
			memEv(detection.EvidenceKilledByNode, "n1"),
			memEv(detection.EvidenceMemoryUsed, "180Mi"),
			memEv(detection.EvidenceMemoryLimit, "512Mi"),
			memEv(detection.EvidenceNodeMemoryUser,
				"batch/importer 3.1Gi (request 1Gi, no limit)"),
			memEv(detection.EvidenceOOMKind, "the node's memory"),
			memEv(detection.EvidenceNodeMemory,
				"requested 7.5Gi of 8Gi allocatable; 7.9Gi in use"),
			memEv(detection.EvidenceOverRequest,
				"used 180Mi, over its 128Mi request"),
		}}
	return Writer{}.Write(announce(singleMember(payments, f)), at(3, 0))
}

func writeEvictionMemory() notification.Message {
	pod := inventory.CoreID(kube.KindPod, "shop", "payments-7d9f")
	f := detection.Finding{Entity: pod, Reason: reasons.Evicted,
		Severity: detection.Warning, Since: at(0, 0),
		Summary: "Pod was evicted",
		Evidence: []detection.Evidence{
			memEv("message", "The node was low on resource: memory."),
			memEv(detection.EvidenceOOMKind, "the node's memory"),
			memEv(detection.EvidenceNodeMemory,
				"requested 7.5Gi of 8Gi allocatable; 7.9Gi in use"),
			memEv(detection.EvidenceOverRequest,
				"used 900Mi, over its 256Mi request"),
			memEv(detection.EvidenceNodeMemoryUser,
				"batch/importer 3.1Gi (request 1Gi, no limit)"),
		}}
	return Writer{}.Write(announce(singleMember(payments, f)), at(3, 0))
}

func writeProbeAtCPULimit() notification.Message {
	f := detection.Finding{Entity: paymentsCt,
		Reason: reasons.ReadinessProbeFailed, Severity: detection.Warning,
		Since: at(0, 0), Summary: "Readiness probe failed",
		Evidence: []detection.Evidence{
			memEv("probe", "Readiness probe failed: Get "+
				"\"http://10.0.0.1:8080/ready\": context deadline exceeded"),
			memEv(detection.EvidenceCPUAtLimit, "248m"),
			memEv(detection.EvidenceCPULimit, "250m"),
			memEv(detection.EvidenceCPUPeak, "250m"),
		}}
	return Writer{}.Write(announce(singleMember(payments, f)), at(3, 0))
}

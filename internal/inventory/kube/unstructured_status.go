package kube

import (
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Attributes of well-known unstructured kinds.
const (
	// AttrListenerProblems lists each Gateway or ListenerSet listener with
	// a failing condition, "listener <name>: <Type>=<Status> (<Reason>)",
	// joined by "; ". The merged conditions keep only the worst per type.
	AttrListenerProblems = "listener.problems"
	// AttrCertificateIssued records whether a CertificateSigningRequest
	// has status.certificate.
	AttrCertificateIssued = "certificate.issued"
	// AttrSignerName is a CertificateSigningRequest's spec.signerName.
	AttrSignerName = "signer.name"
	// AttrAllocated records whether a ResourceClaim has status.allocation.
	AttrAllocated = "allocated"
)

// listenerFailingWhenTrue are listener condition types whose True status
// is the failure; the other listener types fail when False.
var listenerFailingWhenTrue = map[string]bool{
	"Conflicted": true, "OverlappingTLSConfig": true,
}

// conditions returns the merged status conditions, adding the
// per-ancestor conditions policies such as BackendTLSPolicy report under
// status.ancestors[] and the per-device conditions of a ResourceClaim.
func (s UnstructuredSchema) conditions(
	u *unstructured.Unstructured,
) []condition {
	merged := statusConditions(u)
	nested := nestedConditions(u, "ancestors", ancestorName)
	if s.kind == "resourceclaim" {
		nested = append(nested, nestedConditions(u, "devices", deviceName)...)
	}
	for _, c := range nested {
		merged = mergeCondition(merged, c)
	}
	return merged
}

func (s UnstructuredSchema) wellKnownStatus(
	u *unstructured.Unstructured, attrs map[string]inventory.Value,
) {
	switch s.kind {
	case "gateway", "listenerset":
		if problems := listenerProblems(u); problems != "" {
			attrs[AttrListenerProblems] = inventory.Text(
				evidenceText(problems))
		}
	case "certificatesigningrequest":
		certificate, _, _ := unstructured.NestedString(
			u.Object, "status", "certificate")
		attrs[AttrCertificateIssued] = inventory.Bool(certificate != "")
		signer, _, _ := unstructured.NestedString(
			u.Object, "spec", "signerName")
		attrs[AttrSignerName] = inventory.Text(evidenceText(signer))
	case "resourceclaim":
		_, allocated, _ := unstructured.NestedMap(
			u.Object, "status", "allocation")
		attrs[AttrAllocated] = inventory.Bool(allocated)
	}
}

// listenerProblems describes every listener condition that fails, in
// listener and type order.
func listenerProblems(u *unstructured.Unstructured) string {
	entries, _, _ := unstructured.NestedSlice(u.Object, "status", "listeners")
	var out []string
	for _, entry := range entries {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if failing := listenerFailures(m); len(failing) > 0 {
			out = append(out, "listener "+str(m, "name")+": "+
				strings.Join(failing, ", "))
		}
	}
	sort.Strings(out)
	return strings.Join(out, "; ")
}

// listenerFailures labels one listener's failing conditions, sorted.
func listenerFailures(listener map[string]any) []string {
	raw, _, _ := unstructured.NestedSlice(listener, "conditions")
	var failing []string
	for _, c := range parseConditions(raw) {
		bad := "False"
		if listenerFailingWhenTrue[c.Type] {
			bad = "True"
		}
		if c.Status != bad {
			continue
		}
		label := c.Type + "=" + c.Status
		if c.Reason != "" {
			label += " (" + c.Reason + ")"
		}
		failing = append(failing, label)
	}
	sort.Strings(failing)
	return failing
}

func ancestorName(m map[string]any) string {
	name, _, _ := unstructured.NestedString(m, "ancestorRef", "name")
	if name == "" {
		return ""
	}
	return "ancestor " + name
}

func deviceName(m map[string]any) string {
	if device := str(m, "device"); device != "" {
		return "device " + str(m, "pool") + "/" + device
	}
	return ""
}

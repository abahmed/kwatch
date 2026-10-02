package detectors

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Grace periods of the well-known cluster kinds.
const (
	// DefaultCertificateIssue is how long an approved CSR may wait for
	// its signer; kube-controller-manager signs within seconds.
	DefaultCertificateIssue = 5 * time.Minute
	// DefaultServingApproval is how long a kubelet serving CSR may wait
	// for approval. kube-controller-manager never approves them, so an
	// approver must; until then the kubelet has no serving certificate
	// and logs, exec and metrics scraping fail on that node.
	DefaultServingApproval = 15 * time.Minute
	// DefaultClaimUnallocated is how long a pending pod's ResourceClaim
	// may stay unallocated before no device is likely to fit.
	DefaultClaimUnallocated = 5 * time.Minute
)

const kubeletServingSigner = "kubernetes.io/kubelet-serving"

// routeKinds are the Gateway API routes, which send traffic to Services.
var routeKinds = map[inventory.Kind]bool{
	"httproute": true, "grpcroute": true, "tlsroute": true,
	"tcproute": true, "udproute": true,
}

// kindFindings reports failures specific to well-known cluster kinds
// that the shared condition lists cannot name.
func kindFindings(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if routeKinds[e.ID.Kind] {
		return routeBackends(ctx, e)
	}
	var f detection.Finding
	var ok bool
	switch e.ID.Kind {
	case "customresourcedefinition":
		f, ok = crdNotEstablished(ctx, e)
	case "certificatesigningrequest":
		f, ok = certificateRequest(ctx, e)
	case "resourceclaim":
		f, ok = claimUnallocated(ctx, e)
	}
	if !ok {
		return nil
	}
	return []detection.Finding{f}
}

// crdNotEstablished reports a CRD whose API is not served: its names
// conflict with another CRD (NamesAccepted=False) or it is not yet
// Established. Its custom resources cannot be created or read.
func crdNotEstablished(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	for _, conditionType := range []string{"NamesAccepted", "Established"} {
		status, reason, since := condition(e, conditionType)
		if status != "False" || !sustained(ctx, "crd/"+conditionType,
			since, DefaultCustomFailing) {
			continue
		}
		label := conditionType + "=False"
		if reason != "" {
			label += " (" + reason + ")"
		}
		return detection.Finding{
			Reason: reasons.CRDNotEstablished, Severity: detection.Warning,
			Health: detection.Failing, Since: since,
			Summary: "Custom resource API is not served: " + label,
			Evidence: []detection.Evidence{{
				Label: "message", Value: conditionMessage(e, conditionType),
			}},
		}, true
	}
	return detection.Finding{}, false
}

// certificateRequest reports a denied CSR, and one that has no
// certificate although it was approved (no signer for its signerName),
// or, for the kubelet serving signer, has waited too long for approval.
func certificateRequest(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	signer := text(e, kube.AttrSignerName)
	evidence := []detection.Evidence{{Label: "signer", Value: signer}}
	if status, reason, since := condition(e, "Denied"); status == "True" {
		return detection.Finding{
			Reason: reasons.CertificateDenied, Severity: detection.Warning,
			Health: detection.Failing, Since: since,
			Summary:  "Certificate request was denied (" + reason + ")",
			Evidence: evidence,
		}, true
	}
	issued := flag(e, kube.AttrCertificateIssued)
	failed, _, _ := condition(e, "Failed")
	if issued || failed == "True" {
		return detection.Finding{}, false
	}
	approved, _, approvedAt := condition(e, "Approved")
	grace := DefaultCertificateIssue
	since := approvedAt
	summary := "Certificate request was approved but no certificate " +
		"was issued; no signer handles " + signer
	if approved != "True" {
		if signer != kubeletServingSigner {
			return detection.Finding{}, false
		}
		grace, since = DefaultServingApproval, valueSince(e,
			kube.AttrCertificateIssued)
		summary = "Kubelet serving certificate request is waiting for " +
			"approval; kube-controller-manager never approves it"
	}
	if !sustained(ctx, "certificate-not-issued", since, grace) {
		return detection.Finding{}, false
	}
	return detection.Finding{
		Reason: reasons.CertificateNotIssued, Severity: detection.Warning,
		Health: detection.Failing, Since: since, Summary: summary,
		Evidence: evidence,
	}, true
}

// claimUnallocated reports a ResourceClaim without an allocation whose
// owning pod is still pending: the scheduler found no device for it.
// Standalone claims are allocated only when a pod first uses them, so
// they are not judged.
func claimUnallocated(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	if ctx.Model == nil || flag(e, kube.AttrAllocated) {
		return detection.Finding{}, false
	}
	if _, known := e.Attribute(kube.AttrAllocated); !known {
		return detection.Finding{}, false
	}
	var waiting []string
	for _, id := range ctx.Model.Related(
		e.ID, inventory.OwnedBy, inventory.Outgoing,
	) {
		pod, ok := ctx.Model.Entity(id)
		if ok && id.Kind == kube.KindPod &&
			text(pod, kube.AttrPhase) == "Pending" {
			waiting = append(waiting, id.Name)
		}
	}
	since := valueSince(e, kube.AttrAllocated)
	if len(waiting) == 0 || !sustained(ctx, "claim-unallocated", since,
		DefaultClaimUnallocated) {
		return detection.Finding{}, false
	}
	return detection.Finding{
		Reason: reasons.ResourceClaimUnallocated, Severity: detection.Warning,
		Health: detection.Failing, Since: since,
		Summary: "No device has been allocated for " +
			format.Duration(ctx.Now.Sub(since)) + "; pod " +
			strings.Join(waiting, ", ") + " cannot start",
		Evidence: []detection.Evidence{
			{Label: "waiting pod", Value: strings.Join(waiting, ", ")},
		},
	}, true
}

// listenerEvidence lists each failing listener a Gateway or ListenerSet
// reports, one evidence line per listener.
func listenerEvidence(e inventory.Entity) []detection.Evidence {
	problems := text(e, kube.AttrListenerProblems)
	if problems == "" {
		return nil
	}
	var out []detection.Evidence
	for _, problem := range strings.Split(problems, "; ") {
		out = append(out, detection.Evidence{Label: "listener", Value: problem})
	}
	return out
}

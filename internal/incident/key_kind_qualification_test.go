package incident

import (
	"testing"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/event"
)

func TestIncidentOwnerQualifiesWebhookKindByResource(t *testing.T) {
	mutating := event.Event{
		Namespace: "ns", Resource: "mutatingwebhook",
		Reason: constant.ReasonWebhookNoEndpoints,
	}
	validating := event.Event{
		Namespace: "ns", Resource: "validatingwebhook",
		Reason: constant.ReasonWebhookNoEndpoints,
	}

	mutatingKey := BuildKey(
		mutating.Namespace, incidentOwner(mutating, "hook"),
		mutating.Reason, "",
	)
	validatingKey := BuildKey(
		validating.Namespace, incidentOwner(validating, "hook"),
		validating.Reason, "",
	)

	if mutatingKey == validatingKey {
		t.Fatalf("expected different keys, both were %q", mutatingKey)
	}
}

func TestIncidentOwnerQualifiesCustomResourceKindByResource(t *testing.T) {
	certificate := event.Event{
		Namespace: "ns", Resource: "certificate",
		Reason: constant.ReasonCustomResourceFailure,
	}
	issuer := event.Event{
		Namespace: "ns", Resource: "issuer",
		Reason: constant.ReasonCustomResourceFailure,
	}

	certKey := BuildKey(
		certificate.Namespace, incidentOwner(certificate, "shared"),
		certificate.Reason, "",
	)
	issuerKey := BuildKey(
		issuer.Namespace, incidentOwner(issuer, "shared"),
		issuer.Reason, "",
	)

	if certKey == issuerKey {
		t.Fatalf("expected different keys, both were %q", certKey)
	}
}

func TestIncidentOwnerUnqualifiedForOtherReasons(t *testing.T) {
	ev := event.Event{
		Namespace: "ns", Resource: "pod", OwnerKind: "Deployment",
		Reason: "CrashLoopBackOff",
	}

	owner := incidentOwner(ev, "app")
	got := BuildKey(ev.Namespace, owner, ev.Reason, "")
	want := BuildKey(ev.Namespace, "app", ev.Reason, "")

	if got != want {
		t.Fatalf("expected unqualified key %q, got %q", want, got)
	}
}

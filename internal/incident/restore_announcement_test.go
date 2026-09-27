package incident

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/model"
)

func TestRestoredUnannouncedIncidentStaysUnannounced(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	suppressed := &model.Incident{
		Subject: model.Subject{
			Key: "ns:api:CrashLoopBackOff:", Reason: "CrashLoopBackOff",
			Namespace: "ns", Name: "api", Resource: "pod",
		},
		Status:      model.Status{State: model.StateActive},
		Attribution: model.Attribution{SuppressedBy: "mass-failure/x"},
		Delivery:    model.Delivery{NotifiedSig: "firing|x|y"},
	}
	prepareRestoredIncident(suppressed, now)
	if suppressed.NotifiedSig != "" {
		t.Fatalf("never-announced incident restored as announced")
	}

	announced := suppressed.Clone()
	announced.Revision = 2
	prepareRestoredIncident(announced, now)
	if announced.NotifiedSig == "" {
		t.Fatalf("announced incident lost its signature on restore")
	}
}

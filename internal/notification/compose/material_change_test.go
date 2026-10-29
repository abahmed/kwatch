package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
)

// A material change that only raised the tier says so: the restated lead
// alone does not tell what is new.
func TestMaterialChangeSaysTheIncidentIsNowCritical(t *testing.T) {
	p := podCrash(incident.Page)
	p.Delivery.RecordSent(incident.Notify, "n")
	p.Delivery.RecordSent(incident.Page, "p")
	p.Revision = 2
	d := incident.Decision{Action: incident.Update, Incident: p,
		Reason: incident.ReasonMaterialChange}

	note := Writer{}.Write(d, writerNow).Note

	assert.Contains(t, note, "It is now critical.")
}

func TestMaterialChangeWithoutARiseDoesNotClaimOne(t *testing.T) {
	p := podCrash(incident.Page)
	p.Delivery.RecordSent(incident.Page, "p")
	p.Delivery.RecordSent(incident.Page, "p")
	p.Revision = 2
	d := incident.Decision{Action: incident.Update, Incident: p,
		Reason: incident.ReasonMaterialChange}

	note := Writer{}.Write(d, writerNow).Note

	assert.NotContains(t, note, "now critical")
}

// An escalation that arrives with other news still says so: the update
// that reports a worse failure also says the incident is now critical.
func TestWorseningUpdateAlsoSaysTheIncidentIsNowCritical(t *testing.T) {
	p := badRollout()
	crash := crashFinding(detection.ModeCrashLoop)
	p.Members[crash.Key()] = crash
	p.Tier = incident.Page
	p.Delivery.RecordSent(incident.Notify, "n")
	p.Delivery.RecordSent(incident.Page, "p")
	p.Revision = 2
	p.Timeline = []incident.Event{
		{At: at(2, 20), Text: "Pod has not been ready for 3m " +
			"(pod shop/payments-7d9f)"},
		{At: at(7, 0), Text: crash.Summary + " (container shop/" +
			"payments-7d9f/app)"},
	}
	p.Reported, p.ReportedKnown = 1, true
	d := incident.Decision{Action: incident.Update, Incident: p,
		Reason: incident.ReasonMaterialChange}

	note := Writer{}.Write(d, at(7, 0)).Note

	assert.Contains(t, note, "is getting worse")
	assert.Contains(t, note, "It is now critical.")
}

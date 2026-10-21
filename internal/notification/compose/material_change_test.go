package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

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

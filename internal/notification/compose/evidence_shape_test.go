package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
)

// shapeFacts are the three shape facts of one issue.
func shapeFacts() incident.Facts {
	return incident.Facts{ChangesRead: true,
		Grouping: &incident.Grouping{Basis: incident.GroupedNode,
			Text: "Grouped because all 12 pods run on node n1, which " +
				"is not ready."},
		Split: &incident.ReplicaSplit{Lines: []string{"Only the 3 pods " +
			"on arm64 nodes fail; the 4 pods on amd64 nodes are healthy."}},
		Users: &incident.Users{State: incident.UsersServed,
			Text: "Users are still served: route shop.example.com → api " +
				"has 2 of 3 ready endpoints."},
	}
}

// Mail and card providers lay out a message with evidence in their
// evidence form; a shape without a report must not switch them to it.
func TestShapeAloneMakesNoEvidence(t *testing.T) {
	d := uncausedDecision(nil)
	d.Facts = shapeFacts()

	msg := NewWriter("", nil).Write(d, writerNow)

	assert.Nil(t, msg.Evidence)
}

func TestEvidenceCarriesTheShapeBesideTheReport(t *testing.T) {
	d := uncausedDecision(readReport)
	report := d.Facts.Report
	d.Facts = shapeFacts()
	d.Facts.Report = report

	msg := NewWriter("", nil).Write(d, writerNow)

	require.NotNil(t, msg.Evidence)
	assert.Equal(t, 1, msg.Evidence.Checks)
	assert.Equal(t, d.Facts.Grouping.Text, msg.Evidence.Grouped)
	assert.Equal(t, d.Facts.Split.Lines, msg.Evidence.Replicas)
	assert.Equal(t, d.Facts.Users.Text, msg.Evidence.Users)
}

// The shape is evidence only: no chat sentence reads it.
func TestShapeStaysOutOfTheNote(t *testing.T) {
	plain := NewWriter("", nil).Write(uncausedDecision(readReport),
		writerNow)

	d := uncausedDecision(readReport)
	d.Facts = shapeFacts()
	d.Facts.Report = readReport
	shaped := NewWriter("", nil).Write(d, writerNow)
	assert.Equal(t, plain.Note, shaped.Note)
	assert.NotContains(t, shaped.Note, "Grouped because")
}

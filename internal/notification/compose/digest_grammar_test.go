package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestUrgencyLineAgreesWithHowManyProblemsAreOpen(t *testing.T) {
	assert.Empty(t, urgencySentences(0))
	assert.Equal(t, "It is not urgent; if it gets worse it gets its own "+
		"message.", urgencySentences(1)[0].text)
	assert.Equal(t, "None of them is urgent; one that gets worse gets "+
		"its own message.", urgencySentences(2)[0].text)
}

func TestUnchangedLineSpellsOutOne(t *testing.T) {
	web := Ongoing{Root: inventory.CoreID(kube.KindDeployment, "shop", "web")}
	db := Ongoing{Root: inventory.CoreID(kube.KindDeployment, "shop", "db")}

	assert.Equal(t, "one more still failing, unchanged (web)",
		unchangedLine([]Ongoing{web}))
	assert.Equal(t, "two more still failing, unchanged (web, db)",
		unchangedLine([]Ongoing{web, db}))
}

// kwatch is a name that keeps its case at the start of a sentence, as it
// does in every other sentence about itself.
func TestLeadAboutKwatchKeepsItsNameLowercase(t *testing.T) {
	self := kube.KwatchSelf
	p := incident.Incident{ID: "inc-k", Root: self, Tier: incident.Digest,
		State: incident.Open, Opened: at(0, 0), Revision: 1,
		Members: members(detection.Finding{Entity: self,
			Reason: "KwatchProbesFailing", Severity: detection.Info,
			Since: at(0, 0),
			Summary: "kwatch could not reach any of its 5 probed " +
				"dependencies"})}

	title := Writer{}.Write(announce(p), at(6, 0)).Title

	assert.Equal(t, "kwatch could not reach any of its 5 probed "+
		"dependencies.", title)
}

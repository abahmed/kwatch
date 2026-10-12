package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func blamedRelease(change inventory.Change) *Incident {
	deploy := entity(kube.KindDeployment, "rollout")
	p := incidentOf(deploy, nil)
	p.Cause = &rootcause.CauseRecord{Rule: "rollout", Root: deploy,
		Change: &change}
	return p
}

// The blame moving from a deployment's creation record to the spec write
// of the same revision, and the confidence that goes with it, is the same
// release told again.
func TestFingerprintIgnoresTheRecordOfTheSameRevision(t *testing.T) {
	deploy := entity(kube.KindDeployment, "rollout")
	created := blamedRelease(inventory.Change{Entity: deploy,
		Revision: "2", At: at(0), Created: true})
	spec := blamedRelease(inventory.Change{Entity: deploy,
		Revision: "2", At: at(4 * time.Second)})
	created.Cause.Score, spec.Cause.Score = rootcause.High, rootcause.Likely

	assert.Equal(t, fingerprint(created), fingerprint(spec))
}

func TestFingerprintTellsRevisionsAndUnversionedChangesApart(t *testing.T) {
	deploy := entity(kube.KindDeployment, "rollout")
	rev := func(revision string, when time.Duration) string {
		return fingerprint(blamedRelease(inventory.Change{Entity: deploy,
			Revision: revision, At: at(when)}))
	}

	assert.NotEqual(t, rev("2", 0), rev("3", 0), "a new revision is news")
	assert.NotEqual(t, rev("", 0), rev("", time.Minute),
		"without a revision the time tells changes apart")
}

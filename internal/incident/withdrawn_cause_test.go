package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A cause whose evidence ages out while the failure goes on is not a
// new story: the incident moves to its own workload with no cause, and
// a "cause revised" saying nothing is known would only take back what
// the thread said. The thread keeps its words until a different cause
// is found or the incident resolves.
func TestWithdrawnCauseSendsNoRevision(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	node := entity(kube.KindNode, "n1")
	r.cause(web.Entity, node, "node n1 is out of memory")
	r.raise(at(0), web)
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")

	delete(r.rule.causes, web.Entity)
	r.apply(at(5*time.Minute), detection.Changed, web)

	wantNone(t, r.tick(at(5*time.Minute+DefaultReviseSettle)))
	wantNone(t, r.tick(at(7*time.Minute)))
	assert.Equal(t, 1, len(r.m.Export()))
}

// A revision that finds a different cause is still news.
func TestReplacedCauseStillSendsRevision(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	r.cause(web.Entity, entity(kube.KindNode, "n1"), "node n1 is down")
	r.raise(at(0), web)
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")

	r.cause(web.Entity, entity(kube.KindNode, "n2"), "node n2 is down")
	r.apply(at(5*time.Minute), detection.Changed, web)

	wantAction(t, r.tick(at(5*time.Minute+DefaultReviseSettle)),
		Update, ReasonCauseRevised)
}

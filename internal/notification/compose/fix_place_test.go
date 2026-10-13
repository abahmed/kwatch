package compose

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

var placeRoot = inventory.CoreID(kube.KindDeployment, "shop", "api")

// placeCase is an incident rooted at deployment api. The cause is nil
// unless given; the failure is one OOM-killed container whose finding
// holds finding's place.
func placeCase(
	cause *rootcause.CauseRecord, place *detection.Place,
) caseFacts {
	container := inventory.CoreID(kube.KindContainer, "shop", "api-1/app")
	p := incident.Incident{
		ID: "p1", Root: placeRoot, Cause: cause,
		Members: members(detection.Finding{Entity: container,
			Reason: reasons.OOMKilled, Severity: detection.Critical,
			Since: writerNow.Add(-time.Hour), Place: place}),
	}
	return caseFacts{now: writerNow, p: p, members: sortedMembers(p)}
}

func place(container, path, value string) *detection.Place {
	return &detection.Place{Container: container, Path: path, Value: value}
}

func placeLine(f caseFacts) string {
	got := placeSentences(f)
	if len(got) == 0 {
		return ""
	}
	return got[0].text
}

func TestFixLineNamesObjectContainerFieldAndValue(t *testing.T) {
	f := placeCase(nil, place("app", "resources.limits.memory", "256Mi"))

	assert.Equal(t, "Fix in: deployment/api → container app → "+
		"resources.limits.memory (256Mi).", placeLine(f))
}

func TestFixLineLeavesOutWhatIsNotThere(t *testing.T) {
	cause := &rootcause.CauseRecord{Rule: "autoscaling-limit-unavailable",
		Place: &detection.Place{Object: inventory.CoreID(
			kube.KindHPA, "shop", "web"), Path: "spec.maxReplicas",
			Value: "6"}}
	assert.Equal(t, "Fix in: horizontalpodautoscaler/web → "+
		"spec.maxReplicas (6).", placeLine(placeCase(cause, nil)))

	env := place("app", "env[DB_HOST]", "")
	assert.Equal(t, "Fix in: deployment/api → container app → "+
		"env[DB_HOST].", placeLine(placeCase(nil, env)))
}

func TestFixLineComesFromTheCauseWhenThereIsOne(t *testing.T) {
	cause := &rootcause.CauseRecord{Rule: "rollout", Place: &detection.Place{
		Object: placeRoot, Container: "app", Path: "image",
		Value: "api:2"}}
	f := placeCase(cause, place("app", "resources.limits.memory", "256Mi"))

	assert.Equal(t, "Fix in: deployment/api → container app → "+
		"image (api:2).", placeLine(f))
}

func TestFixLineIsAbsentWhenTheCauseIsNotASetting(t *testing.T) {
	cause := &rootcause.CauseRecord{Rule: "node-not-ready"}
	f := placeCase(cause, place("app", "resources.limits.memory", "256Mi"))

	assert.Empty(t, placeLine(f))
}

func TestFixLineIsAbsentWhileTwoCausesAreStated(t *testing.T) {
	cause := &rootcause.CauseRecord{Rule: "rollout",
		Rival: &rootcause.CauseRecord{Rule: "own-change"},
		Place: &detection.Place{Object: placeRoot, Path: "image"}}

	assert.Empty(t, placeLine(placeCase(cause, nil)))
}

func TestFixLineNeedsAWorkloadToPutTheFailuresPlaceIn(t *testing.T) {
	f := placeCase(nil, place("app", "resources.limits.memory", "256Mi"))
	f.p.Root = inventory.CoreID(kube.KindNode, "", "n1")

	assert.Empty(t, placeLine(f))
}

func TestFixLineIsAbsentForAnAdvisory(t *testing.T) {
	f := placeCase(nil, nil)
	for key, m := range f.p.Members {
		m.Place = place("app", "resources.limits.memory", "256Mi")
		m.Advisory = true
		f.p.Members[key] = m
	}
	f.members = sortedMembers(f.p)

	assert.Empty(t, placeLine(f))
}

func TestFixLineReachesTheNote(t *testing.T) {
	f := placeCase(nil, place("app", "resources.limits.memory", "256Mi"))

	note := Writer{}.Write(announce(f.p), writerNow).Note
	assert.Contains(t, note, "Fix in: deployment/api → container app → "+
		"resources.limits.memory (256Mi).")
}

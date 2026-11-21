package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func planeSig(root string) detection.Finding {
	id := kube.Etcd
	if root == "scheduler" {
		id = kube.Scheduler
	}
	return sig(id, "ComponentFailing", detection.Critical)
}

// A component that fails beside the one it rests on is that outage.
func TestControlPlaneFailuresOpenedTogetherShareOneIncident(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), planeSig("etcd"))
	r.raise(at(30*time.Second), planeSig("scheduler"))

	assert.Len(t, r.m.Export(), 1)
	assert.Equal(t, kube.Etcd, r.only().Root)
}

// One that opens long after it is its own news.
func TestControlPlaneFailureLongAfterHasItsOwnIncident(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), planeSig("etcd"))
	r.raise(at(PlaneWindow+time.Minute), planeSig("scheduler"))

	assert.Len(t, r.m.Export(), 2)
}

package explain

import (
	"fmt"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// loggedInPeers is eight healthy workloads that pull from loginRegistry,
// with the named pull Secrets (none when secrets is empty).
func loggedInPeers(f *fixture, secrets string) {
	nodes := f.nodes("zone-a", "n1", "n2")
	for i := 0; i < 8; i++ {
		pods := f.workload("shop", fmt.Sprintf("ok%d", i), 2, nodes...)
		f.pullImage(fmt.Sprintf("%s/team/ok%d:1", loginRegistry, i), pods...)
		for _, pod := range pods {
			attrs := map[string]inventory.Value{
				kube.AttrReady:      inventory.Bool(true),
				kube.AttrReadySince: inventory.Time(t0.Add(-time.Hour))}
			if secrets != "" {
				attrs[kube.AttrPullSecrets] = inventory.Text(secrets)
			}
			f.observe(pod, attrs)
		}
	}
}

// refusedWorkloads adds two workloads that name no pull Secret and are
// refused with a 401, and returns the first failing container.
func refusedWorkloads(f *fixture) inventory.EntityID {
	var first inventory.EntityID
	for _, name := range []string{"newa", "newb"} {
		pod := f.workload("labs", name, 1)[0]
		image := loginRegistry + "/team/" + name + ":1"
		f.pullImage(image, pod)
		f.noteRefused(pod, "Failed to pull image: failed to authorize: "+
			"401 Unauthorized", "unauthorized: authentication required")
		f.fail(containerOf(pod), "ImagePull", failingH, 3,
			"Back-off pulling image "+image)
		if first.IsZero() {
			first = containerOf(pod)
		}
	}
	return first
}

// Pods that send no login while their neighbours log in to the same
// registry fail by their own configuration, not the registry's.
func TestPullWithoutSecretBesideLoginsBlamesTheWorkload(t *testing.T) {
	f := newFixture(t)
	loggedInPeers(f, "regcred")
	effect := refusedWorkloads(f)

	c := requireCause(t, f.explain(), effect, "deployment/labs/newa")
	if c.Row != "pull-without-secret" {
		t.Fatalf("row = %s, want pull-without-secret", c.Row)
	}
}

// Healthy peers alone prove nothing: when they log in with no secret
// either, the registry's refusal stands.
func TestPullWithoutSecretNeedsPeersThatLogIn(t *testing.T) {
	f := newFixture(t)
	loggedInPeers(f, "")
	effect := refusedWorkloads(f)

	requireCause(t, f.explain(), effect, "registry//"+loginRegistry)
}

var pullRowCases = []rowCase{
	{row: "pull-without-secret", want: "deployment/labs/newa",
		build: func(f *fixture) inventory.EntityID {
			loggedInPeers(f, "regcred")
			return refusedWorkloads(f)
		}},
}

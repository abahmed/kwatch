package scenarios

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/replay"
)

// churnReopenCase says where the rescheduled pods land, how long they
// stay not ready and how many other workloads move with the mailer.
type churnReopenCase struct {
	// others are the workloads besides the mailer, at most api and web.
	others int
	// toFresh sends them to a new node; otherwise to the settled n2.
	toFresh bool
	// blip is how long the moved pods are not ready.
	blip time.Duration
}

// buildChurnAfterNotify: the mailer crash-looped, was fixed and has been
// healthy for six minutes, so its notify incident can still reopen. Then
// the spot node n1 is reclaimed; the pods of mailer, api and web move.
func buildChurnAfterNotify(c *cluster, tc churnReopenCase) {
	n1, n2 := c.node("n1", "zone-a"), c.node("n2", "zone-a")
	c.list(n1, n2)
	mailer := c.deployment("shop", "mailer",
		"registry.example.com/mailer:2", 1)
	api := c.deployment("shop", "api", "registry.example.com/api:5", 1)
	web := c.deployment("shop", "web", "registry.example.com/web:9", 1)
	all := append([]*workload{mailer}, []*workload{api, web}[:tc.others]...)
	for _, w := range all {
		c.list(w.objects())
		c.list(w.pod(0, "n1"))
	}
	c.after(time.Minute)
	crashFor(c, mailer, "panic: mailer: smtp relay rejected credentials",
		10*time.Minute)
	recoverFor(c, mailer, 6*time.Minute)

	dest := "n2"
	if tc.toFresh {
		n3 := c.node("n3", "zone-a")
		n3.CreationTimestamp = metav1.NewTime(c.now)
		c.create(n3)
		dest = "n3"
	}
	c.remove(n1)
	for _, w := range all {
		c.remove(w.pod(0, "n1"))
		w.setReady(0)
		c.update(w.objects())
		c.create(w.pod(1, dest, startedNow, notReady))
	}
	c.after(tc.blip)
	for _, w := range all {
		c.update(w.pod(1, dest, startedNow))
		w.setReady(1)
		c.update(w.objects())
	}
	c.after(10 * time.Minute)
}

func replayChurn(t *testing.T, tc churnReopenCase) replay.Result {
	t.Helper()
	c := newCluster(scenarioStart, "")
	buildChurnAfterNotify(c, tc)
	return replayLog(t, c.log(), replay.Options{Tail: 10 * time.Minute})
}

// Churn is the same on a new node and on a settled one, with or without
// the other workloads: nothing says "failing again".
func TestNodeChurnBlipDoesNotReopenANotifyIncident(t *testing.T) {
	for name, tc := range map[string]churnReopenCase{
		"fresh node":   {toFresh: true, others: 2, blip: 2 * time.Minute},
		"settled node": {others: 2, blip: 2 * time.Minute},
		"four minutes": {others: 2, blip: 4 * time.Minute},
		"mailer alone": {blip: 4 * time.Minute},
	} {
		t.Run(name, func(t *testing.T) {
			result := replayChurn(t, tc)
			assert.Empty(t, decisionsOf(result, incident.Update,
				incident.ReasonFailingAgain), "no failing again")
			assert.Len(t, result.Messages, 2,
				"the mailer's own announcement and resolve only")
		})
	}
}

// Pods that keep failing on the new node are reported once the start
// time is over, as an incident of their own.
func TestNodeChurnLongFailureIsReportedAsNew(t *testing.T) {
	result := replayChurn(t, churnReopenCase{blip: 15 * time.Minute})
	assert.Empty(t, decisionsOf(result, incident.Update,
		incident.ReasonFailingAgain), "not the old incident again")
	announced := decisionsOf(result, incident.Announce, "")
	require.Len(t, announced, 2, "the crash, then the long failure")
	assert.NotEqual(t, announced[0].Incident.ID, announced[1].Incident.ID)
}

// spotReclaimAfterNotify: the mailer's notify incident resolved six
// minutes ago when its spot node is reclaimed. Three workloads move to
// another node and are not ready for four minutes. The pods only moved,
// so the incident does not reopen and nothing else is said.
func spotReclaimAfterNotify() scenario {
	return scenario{
		expect: expectation{
			Name: "spot-reclaim-after-notify",
			Description: "A spot node is reclaimed six minutes after " +
				"a notify incident resolved; its workloads are not " +
				"ready for four minutes on the new node.",
			Root: "deployment/shop/mailer", Tier: "notify",
			MaxMessages: 2,
			Tail:        duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			buildChurnAfterNotify(c, churnReopenCase{
				others: 2, blip: 4 * time.Minute})
		},
	}
}

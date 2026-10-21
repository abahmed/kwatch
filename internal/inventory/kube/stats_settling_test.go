package kube

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
)

// pollRounds polls n times and returns every round's summary.
func pollRounds(
	n int, failures map[string]error,
	settling func(inventory.EntityID, time.Time) bool,
) []StatsRound {
	var rounds []StatsRound
	p := NewStatsPoller(StatsConfig{
		Kubelet:  mapKubelet{failures: failures},
		Now:      fixedNow,
		Submit:   func(context.Context, ...inventory.Observation) {},
		Nodes:    statsNodes("a", "b", "c"),
		Settling: settling,
		Report:   func(r StatsRound) { rounds = append(rounds, r) },
	})
	for i := 0; i < n; i++ {
		p.poll(context.Background())
	}
	return rounds
}

var tlsNotReady = errors.New("remote error: tls: internal error")

func TestStatsPollerWaitsForASecondFailureBeforeDegrading(t *testing.T) {
	rounds := pollRounds(3, map[string]error{"b": tlsNotReady}, nil)

	assert.Equal(t, StatsRound{Nodes: 3, Failed: 1}, rounds[0],
		"one failed poll is a blip, not a reason")
	assert.Equal(t, StatsReasonPartial, rounds[1].Reason)
	assert.Equal(t, StatsReasonPartial, rounds[2].Reason)
}

func TestStatsPollerNeverDegradesForASettlingNode(t *testing.T) {
	settling := func(node inventory.EntityID, _ time.Time) bool {
		return node.Name == "b" // a node two minutes old
	}

	rounds := pollRounds(4, map[string]error{"b": tlsNotReady}, settling)

	for i, round := range rounds {
		assert.Empty(t, round.Reason, "round %d", i)
		assert.Equal(t, 1, round.Failed, "the failure is still counted")
	}
}

func TestStatsPollerStillDegradesForAnotherNodeNextToASettlingOne(
	t *testing.T,
) {
	settling := func(node inventory.EntityID, _ time.Time) bool {
		return node.Name == "b"
	}
	failures := map[string]error{"b": tlsNotReady, "c": tlsNotReady}

	rounds := pollRounds(2, failures, settling)

	assert.Equal(t, StatsReasonPartial, rounds[1].Reason,
		"node c is not settling and failed twice in a row")
	assert.Equal(t, 2, rounds[1].Failed)
}

func TestStatsPollerRefusalDegradesAtOnceEvenOnASettlingNode(t *testing.T) {
	settling := func(inventory.EntityID, time.Time) bool { return true }

	rounds := pollRounds(1, map[string]error{"a": codedError{403}}, settling)

	assert.Equal(t, StatsReasonPermissionDenied, rounds[0].Reason)
}

func TestNodeSettlingIsYoungOrCordoned(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	model := inventory.NewModel(inventory.Options{})
	node := func(
		name string, age time.Duration, cordoned bool,
	) inventory.EntityID {
		id := inventory.CoreID(KindNode, "", name)
		_, err := model.Apply(inventory.Observation{
			Kind: inventory.Observed, Source: "k", At: now, Entity: id,
			Attributes: map[string]inventory.Value{
				AttrCreated:       inventory.Time(now.Add(-age)),
				AttrUnschedulable: inventory.Bool(cordoned),
			},
		})
		assert.NoError(t, err)
		return id
	}

	assert.True(t, NodeSettling(model, node("young", 2*time.Minute, false),
		now), "a node two minutes old")
	assert.True(t, NodeSettling(model, node("cordoned", time.Hour, true),
		now), "a node being drained")
	assert.False(t, NodeSettling(model, node("steady", time.Hour, false),
		now))
	assert.False(t, NodeSettling(model, inventory.CoreID(KindNode, "",
		"unknown"), now))
}

package kube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
)

// codedError is a kubelet refusal carrying an HTTP status.
type codedError struct{ code int }

func (e codedError) Error() string   { return fmt.Sprintf("status %d", e.code) }
func (e codedError) StatusCode() int { return e.code }

// mapKubelet answers each node's summary with "{}" unless failures names
// an error for it. It records the order of summary reads.
type mapKubelet struct {
	failures map[string]error
	order    chan string
}

func (k mapKubelet) Open(
	_ context.Context, node, path string,
) (io.ReadCloser, error) {
	if path == "stats/summary" && k.order != nil {
		k.order <- node
	}
	if err := k.failures[node]; err != nil {
		return nil, err
	}
	if path != "stats/summary" {
		return nil, errors.New("not served")
	}
	return io.NopCloser(strings.NewReader("{}")), nil
}

func statsNodes(names ...string) func() []inventory.EntityID {
	return func() []inventory.EntityID {
		ids := make([]inventory.EntityID, 0, len(names))
		for _, name := range names {
			ids = append(ids, inventory.CoreID(KindNode, "", name))
		}
		return ids
	}
}

func roundFor(t *testing.T, failures map[string]error) StatsRound {
	t.Helper()
	var rounds []StatsRound
	p := NewStatsPoller(StatsConfig{
		Kubelet: mapKubelet{failures: failures},
		Now:     fixedNow,
		Submit:  func(context.Context, ...inventory.Observation) {},
		Nodes:   statsNodes("a", "b", "c"),
		Report:  func(r StatsRound) { rounds = append(rounds, r) },
	})
	p.poll(context.Background())
	require.Len(t, rounds, 1)
	return rounds[0]
}

func fixedNow() time.Time {
	return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
}

func TestStatsPollerReportsReachability(t *testing.T) {
	down := errors.New("dial tcp: connection refused")
	cases := []struct {
		name     string
		failures map[string]error
		want     StatsRound
	}{
		{"every node answers", nil, StatsRound{Nodes: 3}},
		{"one node unreachable", map[string]error{"b": down},
			StatsRound{Nodes: 3, Failed: 1, Reason: StatsReasonPartial}},
		{"no node answers",
			map[string]error{"a": down, "b": down, "c": down},
			StatsRound{Nodes: 3, Failed: 3, Reason: StatsReasonUnreachable}},
		{"kubelet refuses the request",
			map[string]error{"a": fmt.Errorf("read: %w", codedError{403})},
			StatsRound{
				Nodes: 3, Failed: 1, Reason: StatsReasonPermissionDenied,
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, roundFor(t, tc.failures))
		})
	}
}

func TestStatsPollerReportsMissingClient(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	rounds := make(chan StatsRound, 1)
	p := NewStatsPoller(StatsConfig{
		Now: fixedNow, Nodes: statsNodes("a"),
		Report: func(r StatsRound) { rounds <- r },
	})
	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()

	assert.Equal(t, StatsRound{Reason: StatsReasonNotConfigured}, <-rounds)
	cancel()
	<-done
}

func TestStatsPollerRotatesPastUnreachedNodes(t *testing.T) {
	p := NewStatsPoller(StatsConfig{})
	nodes := statsNodes("a", "b", "c")()

	assert.Equal(t, nodes, p.rotate(nodes))
	// The round started only a and b: the next one begins at c.
	p.advance(len(nodes), 2)
	assert.Equal(t, []string{"c", "a", "b"}, names(p.rotate(nodes)))
	// A complete round keeps the start, so no node is favoured.
	p.advance(len(nodes), 3)
	assert.Equal(t, []string{"c", "a", "b"}, names(p.rotate(nodes)))
	// A shrinking node list keeps the offset in range.
	p.advance(1, 0)
	assert.Len(t, p.rotate(nodes[:1]), 1)
	p.advance(0, 0)
	assert.Empty(t, p.rotate(nil))
}

func TestStatsPollerPollsInRotatedOrder(t *testing.T) {
	order := make(chan string, 3)
	p := NewStatsPoller(StatsConfig{
		Kubelet: mapKubelet{order: order},
		Now:     fixedNow,
		Submit:  func(context.Context, ...inventory.Observation) {},
		Nodes:   statsNodes("a"),
	})
	p.next = 5

	p.poll(context.Background())

	assert.Equal(t, "a", <-order)
	assert.Equal(t, 0, p.next)
}

func TestStatsPollerRateLimitsFailureLogs(t *testing.T) {
	now := fixedNow()
	p := NewStatsPoller(StatsConfig{
		Kubelet: mapKubelet{failures: map[string]error{
			"a": errors.New("down"),
		}},
		Now:    func() time.Time { return now },
		Submit: func(context.Context, ...inventory.Observation) {},
		Nodes:  statsNodes("a"),
	})

	p.poll(context.Background())
	first := p.lastLog
	now = now.Add(statsLogEvery - time.Second)
	p.poll(context.Background())
	assert.Equal(t, first, p.lastLog, "logged again within the window")
	now = now.Add(time.Second)
	p.poll(context.Background())
	assert.Equal(t, now, p.lastLog)
	assert.True(t, p.failing)

	p.cfg.Kubelet = mapKubelet{}
	p.poll(context.Background())
	assert.False(t, p.failing, "recovery not recorded")
}

func TestStatsPollerCountsUndecodableSummary(t *testing.T) {
	var got StatsRound
	p := NewStatsPoller(StatsConfig{
		Kubelet: bodyKubelet{http.HandlerFunc(
			func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("not json"))
			})},
		Now:    fixedNow,
		Submit: func(context.Context, ...inventory.Observation) {},
		Nodes:  statsNodes("a"),
		Report: func(r StatsRound) { got = r },
	})

	p.poll(context.Background())

	assert.Equal(t, StatsRound{
		Nodes: 1, Failed: 1, Reason: StatsReasonUnreachable,
	}, got)
}

func names(ids []inventory.EntityID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.Name)
	}
	return out
}

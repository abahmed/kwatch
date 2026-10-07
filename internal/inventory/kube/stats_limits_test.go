package kube

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
)

// bodyKubelet answers every kubelet read with the handler's response.
type bodyKubelet struct{ h http.Handler }

func (k bodyKubelet) Open(
	_ context.Context, _, path string,
) (io.ReadCloser, error) {
	rec := httptest.NewRecorder()
	k.h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/"+path, nil))
	return io.NopCloser(rec.Body), nil
}

func TestStatsPollerRejectsOversizeBody(t *testing.T) {
	kubelet := bodyKubelet{http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(strings.Repeat("x", 11)))
		})}
	p := NewStatsPoller(StatsConfig{Kubelet: kubelet})
	node := inventory.CoreID(KindNode, "", "n1")

	_, err := p.read(context.Background(), node, "metrics", 10)
	assert.ErrorIs(t, err, errBodyTooLarge)

	body, err := p.read(context.Background(), node, "metrics", 11)
	require.NoError(t, err)
	assert.Len(t, body, 11)
}

func TestStatsPollerSkipsNodeWithOversizeSummary(t *testing.T) {
	big := strings.Repeat(" ", maxSummaryBytes+1)
	kubelet := bodyKubelet{http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("{}" + big))
		})}
	submitted := false
	p := NewStatsPoller(StatsConfig{
		Kubelet: kubelet, Now: func() time.Time { return time.Unix(0, 0) },
		Submit: func(_ context.Context, obs ...inventory.Observation) {
			for _, o := range obs {
				if _, ok := o.Attributes[AttrKubeletFailures]; !ok {
					submitted = true
				}
			}
		},
		Nodes: func() []inventory.EntityID {
			return []inventory.EntityID{inventory.CoreID(KindNode, "", "n1")}
		},
	})

	p.poll(context.Background())

	assert.False(t, submitted, "oversize summary must be dropped")
}

func TestStatsPollerForgetsSamplesOfVanishedEntities(t *testing.T) {
	now := time.Unix(0, 0)
	p := NewStatsPoller(StatsConfig{
		Interval: time.Minute,
		Now:      func() time.Time { return now },
		Nodes:    func() []inventory.EntityID { return nil },
	})
	p.counters.rate("network/n1", now, 1)

	now = now.Add(5 * time.Minute)
	p.poll(context.Background())
	assert.Len(t, p.counters.previous, 1, "kept within the TTL")

	now = now.Add(time.Hour)
	p.poll(context.Background())
	assert.Empty(t, p.counters.previous)
}

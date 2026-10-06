package kube_test

import (
	"context"
	"errors"
	"net"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// podCalling builds a model whose pods call the named endpoints.
func podCalling(t *testing.T, names ...string) inventory.Reader {
	t.Helper()
	model := inventory.NewModel(inventory.Options{})
	pod := inventory.CoreID(kube.KindPod, "shop", "api")
	_, err := model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: kube.ObservationSource,
		At: fixedTime(), Entity: pod,
		Attributes: map[string]inventory.Value{},
	})
	require.NoError(t, err)
	var targets []inventory.EntityID
	for _, name := range names {
		targets = append(targets,
			inventory.CoreID(kube.KindExternalEndpoint, "", name))
	}
	_, err = model.Apply(inventory.Observation{
		Kind: inventory.Related, Source: kube.ObservationSource,
		At: fixedTime(), Entity: pod, Relation: inventory.Calls,
		Targets: targets,
	})
	require.NoError(t, err)
	return model
}

func failWith(err error) func(
	context.Context, string, string,
) (net.Conn, error) {
	return func(context.Context, string, string) (net.Conn, error) {
		return nil, err
	}
}

func TestActiveProberRecordsHowADependencyFailed(t *testing.T) {
	db := inventory.CoreID(kube.KindExternalEndpoint, "", "db.example.com:5432")
	refused := &net.OpError{Op: "dial", Err: os.NewSyscallError("connect",
		syscall.ECONNREFUSED)}
	noName := &net.OpError{Op: "dial", Err: &net.DNSError{
		Err: "no such host", Name: "db.example.com", IsNotFound: true}}
	serverFail := &net.OpError{Op: "dial", Err: &net.DNSError{
		Err: "server misbehaving", Name: "db.example.com"}}
	cases := map[string]struct {
		err  error
		want string
	}{
		"dns_server_failure": {serverFail, kube.FailureDNSLookup},
		"silent_target_is_a_timeout": {context.DeadlineExceeded,
			kube.FailureTimeout},
		"refusal":                 {refused, kube.FailureRefused},
		"missing_name":            {noName, kube.FailureDNS},
		"unrecognised_is_unnamed": {errors.New("boom"), ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			observations := runActive(t, kube.ActiveProbeConfig{
				AutoDependencies: true, Model: podCalling(t, db.Name),
				Timeout: 3 * time.Second, Dial: failWith(c.err),
			})

			attrs := observations[db].Attributes
			assert.Equal(t, c.want, attrs[kube.AttrProbeFailureKind].AsText())
			wait, _ := attrs[kube.AttrProbeTimeoutSeconds].AsNumber()
			assert.Equal(t, 3.0, wait)
		})
	}
}

func TestActiveProberBlamesItsOwnNetworkWhenEveryDependencyFails(t *testing.T) {
	model := podCalling(t, "a.example.com:443", "b.example.com:443",
		"c.example.com:443")

	observations := runActive(t, kube.ActiveProbeConfig{
		AutoDependencies: true, Model: model,
		Dial: failWith(context.DeadlineExceeded),
	})

	require.Len(t, observations, 1, "no dependency is reported as down")
	self := observations[kube.KwatchSelf]
	count, _ := self.Attributes[kube.AttrDependenciesUnreachable].AsNumber()
	assert.Equal(t, 3.0, count)
}

func TestActiveProberReportsEachDependencyWhenItsCallersAreFailing(
	t *testing.T,
) {
	model := podCalling(t, "a.example.com:443", "b.example.com:443",
		"c.example.com:443")
	_, err := model.(*inventory.Model).Apply(inventory.Observation{
		Kind: inventory.Observed, Source: kube.ObservationSource,
		At: fixedTime(), Entity: inventory.CoreID(kube.KindPod, "shop", "api"),
		Attributes: map[string]inventory.Value{
			kube.AttrReady: inventory.Bool(false)},
	})
	require.NoError(t, err)

	observations := runActive(t, kube.ActiveProbeConfig{
		AutoDependencies: true, Model: model,
		Dial: failWith(context.DeadlineExceeded),
	})

	assert.Len(t, observations, 3, "a real outage is not hidden")
	assert.NotContains(t, observations, kube.KwatchSelf)
}

func TestActiveProberKeepsPerDependencyFailuresWhenOneAnswers(t *testing.T) {
	model := podCalling(t, "a.example.com:443", "b.example.com:443",
		"c.example.com:443")
	dial := func(_ context.Context, _, address string) (net.Conn, error) {
		if address == "c.example.com:443" {
			client, server := net.Pipe()
			_ = server.Close()
			return client, nil
		}
		return nil, context.DeadlineExceeded
	}

	observations := runActive(t, kube.ActiveProbeConfig{
		AutoDependencies: true, Model: model, Dial: dial,
	})

	assert.Len(t, observations, 3)
	assert.NotContains(t, observations, kube.KwatchSelf)
}

func TestActiveProberIgnoresTooFewDependenciesForTheGuard(t *testing.T) {
	model := podCalling(t, "a.example.com:443", "b.example.com:443")

	observations := runActive(t, kube.ActiveProbeConfig{
		AutoDependencies: true, Model: model,
		Dial: failWith(context.DeadlineExceeded),
	})

	assert.Len(t, observations, 2)
	assert.NotContains(t, observations, kube.KwatchSelf)
}

func TestActiveProberClearsTheNetworkAdvisoryOnceOneAnswers(t *testing.T) {
	model := podCalling(t, "a.example.com:443", "b.example.com:443",
		"c.example.com:443")
	var calls atomic.Int32
	dial := func(context.Context, string, string) (net.Conn, error) {
		if calls.Add(1) <= 3 {
			return nil, context.DeadlineExceeded
		}
		client, server := net.Pipe()
		_ = server.Close()
		return client, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rounds := make(chan []inventory.Observation)
	prober := kube.NewActiveProber(kube.ActiveProbeConfig{
		AutoDependencies: true, Model: model, Dial: dial,
		Interval: time.Millisecond, Now: fixedTime,
		Submit: func(_ context.Context, o ...inventory.Observation) {
			select {
			case rounds <- o:
			case <-ctx.Done():
			}
		},
	})
	done := make(chan struct{})
	go func() { prober.Run(ctx); close(done) }()

	first, second := <-rounds, <-rounds
	cancel()
	<-done

	assert.Len(t, first, 1)
	assert.Len(t, second, 4, "three dependencies and the cleared advisory")
	count := -1.0
	for _, o := range second {
		if o.Entity == kube.KwatchSelf {
			count, _ = o.Attributes[kube.AttrDependenciesUnreachable].
				AsNumber()
		}
	}
	assert.Equal(t, 0.0, count)
}

// withContainer adds a container of the pod "api" with these attributes.
func withContainer(
	t *testing.T, model inventory.Reader, attrs map[string]inventory.Value,
) {
	t.Helper()
	container := inventory.CoreID(kube.KindContainer, "shop", "api/main")
	m := model.(*inventory.Model)
	_, err := m.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: kube.ObservationSource,
		At: fixedTime(), Entity: container, Attributes: attrs,
	})
	require.NoError(t, err)
	_, err = m.Apply(inventory.Observation{
		Kind: inventory.Related, Source: kube.ObservationSource,
		At: fixedTime(), Entity: container, Relation: inventory.PartOf,
		Targets: []inventory.EntityID{
			inventory.CoreID(kube.KindPod, "shop", "api")},
	})
	require.NoError(t, err)
}

func probeAll(t *testing.T, model inventory.Reader,
) map[inventory.EntityID]inventory.Observation {
	return runActive(t, kube.ActiveProbeConfig{
		AutoDependencies: true, Model: model,
		Dial: failWith(context.DeadlineExceeded),
	})
}

func TestOldRestartsDoNotMakeACallerLookBroken(t *testing.T) {
	model := podCalling(t, "a.example.com:443", "b.example.com:443",
		"c.example.com:443")
	withContainer(t, model, map[string]inventory.Value{
		kube.AttrRestarts: inventory.Number(7),
		kube.AttrState:    inventory.Text("running"),
		kube.AttrLastFinished: inventory.Time(
			fixedTime().Add(-48 * time.Hour)),
	})

	observations := probeAll(t, model)

	assert.Contains(t, observations, kube.KwatchSelf,
		"a restart two days ago says nothing about the dependency")
}

func TestARecentRestartMakesACallerLookBroken(t *testing.T) {
	model := podCalling(t, "a.example.com:443", "b.example.com:443",
		"c.example.com:443")
	withContainer(t, model, map[string]inventory.Value{
		kube.AttrRestarts: inventory.Number(7),
		kube.AttrState:    inventory.Text("waiting"),
		kube.AttrLastFinished: inventory.Time(
			fixedTime().Add(-2 * time.Minute)),
	})

	observations := probeAll(t, model)

	assert.NotContains(t, observations, kube.KwatchSelf)
}

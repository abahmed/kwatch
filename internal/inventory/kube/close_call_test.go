package kube_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

type closeResolver struct {
	addrs []string
	err   error
	calls atomic.Int32
}

func (r *closeResolver) LookupHost(context.Context, string) ([]string, error) {
	r.calls.Add(1)
	return r.addrs, r.err
}

func dialing(
	err error,
) func(context.Context, string, string) (net.Conn, error) {
	return func(context.Context, string, string) (net.Conn, error) {
		if err != nil {
			return nil, err
		}
		client, server := net.Pipe()
		_ = server.Close()
		return client, nil
	}
}

func TestCloseCallSeparatesDNSFromTheEndpoint(t *testing.T) {
	now := time.Unix(100, 0)
	clock := func() time.Time { return now }
	refused := errors.New("dial tcp 203.0.113.9:5432: connect: connection refused")
	tt := []struct {
		name     string
		resolver *closeResolver
		dial     error
		check    func(*testing.T, kube.CloseCallResult)
	}{
		{"resolves_and_refuses", &closeResolver{addrs: []string{"203.0.113.9"}},
			refused, func(t *testing.T, r kube.CloseCallResult) {
				assert.True(t, r.Resolved)
				assert.True(t, r.Refused)
				assert.False(t, r.Connected)
			}},
		{"resolves_and_connects", &closeResolver{addrs: []string{"203.0.113.9"}},
			nil, func(t *testing.T, r kube.CloseCallResult) {
				assert.True(t, r.Connected)
			}},
		{"does_not_resolve", &closeResolver{err: &net.DNSError{
			Err: "no such host", IsNotFound: true}}, nil,
			func(t *testing.T, r kube.CloseCallResult) {
				assert.False(t, r.Resolved)
				assert.Equal(t, "no such host", r.Error)
				assert.False(t, r.TimedOut)
			}},
		{"lookup_times_out", &closeResolver{err: context.DeadlineExceeded}, nil,
			func(t *testing.T, r kube.CloseCallResult) {
				assert.True(t, r.TimedOut, "inconclusive, not a verdict")
			}},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			check := kube.NewCloseCall(tc.resolver, dialing(tc.dial), clock)
			tc.check(t, check.Check(context.Background(),
				"db.example.com:5432"))
		})
	}
}

func TestCloseCallDoesNotRepeatARecentCheck(t *testing.T) {
	now := time.Unix(100, 0)
	resolver := &closeResolver{addrs: []string{"203.0.113.9"}}
	check := kube.NewCloseCall(resolver, dialing(nil),
		func() time.Time { return now })
	check.Check(context.Background(), "db.example.com:5432")
	now = now.Add(time.Minute)
	check.Check(context.Background(), "db.example.com:5432")
	assert.EqualValues(t, 1, resolver.calls.Load())
	now = now.Add(2 * time.Minute)
	check.Check(context.Background(), "db.example.com:5432")
	assert.EqualValues(t, 2, resolver.calls.Load())
}

func TestCloseCallRefusesAPrivateAddress(t *testing.T) {
	resolver := &closeResolver{addrs: []string{"10.0.0.7"}}
	check := kube.NewCloseCall(resolver, nil, time.Now)
	r := check.Check(context.Background(), "10.0.0.7:5432")
	assert.True(t, r.Blocked || !r.Connected)
	assert.False(t, r.Connected)
}

func TestCloseCallDoesNotRememberACheckTheCallerCutShort(t *testing.T) {
	resolver := &closeResolver{err: context.Canceled}
	check := kube.NewCloseCall(resolver, dialing(nil),
		func() time.Time { return time.Unix(100, 0) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	check.Check(ctx, "db.example.com:5432")
	check.Check(context.Background(), "db.example.com:5432")
	assert.EqualValues(t, 2, resolver.calls.Load())
}

func TestCloseCallSharesOneDeadlineAcrossLookupAndConnect(t *testing.T) {
	var lookup, connect time.Time
	resolver := &deadlineResolver{at: &lookup}
	dial := func(ctx context.Context, _, _ string) (net.Conn, error) {
		connect, _ = ctx.Deadline()
		return nil, errors.New("connection refused")
	}
	check := kube.NewCloseCall(resolver, dial, time.Now)
	check.Check(context.Background(), "db.example.com:5432")
	assert.False(t, lookup.IsZero())
	assert.Equal(t, lookup, connect)
	assert.WithinDuration(t, time.Now().Add(kube.CloseCallTimeout),
		connect, time.Second)
}

type deadlineResolver struct{ at *time.Time }

func (r *deadlineResolver) LookupHost(
	ctx context.Context, _ string,
) ([]string, error) {
	*r.at, _ = ctx.Deadline()
	return []string{"203.0.113.9"}, nil
}

func TestCloseCallEvictsOnlyTheOldestWhenFull(t *testing.T) {
	now := time.Unix(100, 0)
	resolver := &closeResolver{addrs: []string{"203.0.113.9"}}
	check := kube.NewCloseCall(resolver, dialing(nil),
		func() time.Time { return now })
	for i := 0; i < 64; i++ {
		check.Check(context.Background(), fmt.Sprintf("h%d.example.com:1", i))
		now = now.Add(time.Second)
	}
	check.Check(context.Background(), "new.example.com:1")
	assert.EqualValues(t, 65, resolver.calls.Load())
	// Everything but the oldest is still remembered.
	check.Check(context.Background(), "h1.example.com:1")
	check.Check(context.Background(), "h63.example.com:1")
	assert.EqualValues(t, 65, resolver.calls.Load())
	check.Check(context.Background(), "h0.example.com:1")
	assert.EqualValues(t, 66, resolver.calls.Load())
}

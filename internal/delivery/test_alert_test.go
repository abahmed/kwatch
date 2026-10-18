package delivery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/notification"
)

type testAlertProvider struct {
	name  string
	err   error
	block bool
	pages bool
	got   chan string
}

func (p *testAlertProvider) Name() string { return p.name }

func (p *testAlertProvider) SendMessage(
	ctx context.Context, text string,
) error {
	p.got <- text
	if p.block {
		<-ctx.Done()
		return ctx.Err()
	}
	return p.err
}

func (p *testAlertProvider) SendIncident(
	context.Context, notification.Message,
) error {
	return errors.New("a test alert is never an incident")
}

func (p *testAlertProvider) SkipsPlainMessages() bool { return p.pages }

func newTestAlertProvider(name string) *testAlertProvider {
	return &testAlertProvider{name: name, got: make(chan string, 1)}
}

func TestManagerSendTestAlertReportsEachProvider(t *testing.T) {
	ok := newTestAlertProvider("slack")
	bad := newTestAlertProvider("discord")
	bad.err = errors.New("status 404")
	slow := newTestAlertProvider("teams")
	slow.block = true
	pager := newTestAlertProvider("pagerduty")
	pager.pages = true
	m := managerWithEntries([]providerEntry{
		{provider: ok}, {provider: bad}, {provider: slow},
		{provider: pager},
	})

	results := m.SendTestAlert(context.Background(), "", "hello",
		20*time.Millisecond)

	require.Len(t, results, 4)
	require.Equal(t, TestSent, results[0].Outcome)
	require.Equal(t, TestFailed, results[1].Outcome)
	require.EqualError(t, results[1].Err, "status 404")
	require.Equal(t, TestFailed, results[2].Outcome)
	require.ErrorIs(t, results[2].Err, context.DeadlineExceeded)
	require.Equal(t, TestSkipped, results[3].Outcome)
	require.NotEmpty(t, results[3].Reason)
	require.Empty(t, pager.got, "a paging provider is never sent a test")
	require.Equal(t, "hello", <-ok.got)
	require.Equal(t, []string{"slack", "discord", "teams", "pagerduty"},
		m.ProviderNames())
}

func TestManagerSendTestAlertToOneProvider(t *testing.T) {
	a, b := newTestAlertProvider("a"), newTestAlertProvider("b")
	m := managerWithEntries([]providerEntry{{provider: a}, {provider: b}})

	results := m.SendTestAlert(context.Background(), "b", "x", time.Second)

	require.Len(t, results, 1)
	require.Equal(t, "b", results[0].Provider)
	require.Empty(t, a.got)
	require.Empty(t, newTestManager().SendTestAlert(
		context.Background(), "", "x", time.Second))
}

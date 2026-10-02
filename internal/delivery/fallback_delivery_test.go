package delivery

import (
	"context"
	"testing"
)

func TestNotifyIncidentAfterShutdownIsNoop(t *testing.T) {
	am := newTestManager()
	setManagerEntries(am, []providerEntry{{
		provider: &fakeProvider{},
		retry:    retryConfig{maxAttempts: 1},
		ch:       make(chan deliverJob, channelCap),
	}})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	am.Start(ctx)
	shutdownManager(am)

	am.NotifyIncident(*incidentJob("k", "default").incident)
}

func TestManagerCanRestartAfterShutdown(t *testing.T) {
	am := newTestManager()
	setManagerEntries(am, []providerEntry{{
		provider: &fakeProvider{},
		retry:    retryConfig{maxAttempts: 1},
		ch:       make(chan deliverJob, channelCap),
	}})

	ctx1, cancel1 := context.WithCancel(context.Background())
	am.Start(ctx1)
	shutdownManager(am)
	cancel1()

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	am.Start(ctx2)
	am.NotifyIncident(*incidentJob("k", "default").incident)
	shutdownManager(am)
}

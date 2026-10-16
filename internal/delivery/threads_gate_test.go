package delivery

import (
	"testing"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/feature"
)

type switchableProvider struct {
	errorRecorderProvider
	threadsOff bool
}

func (p *switchableProvider) DisableThreads() { p.threadsOff = true }

func initThreaded(
	t *testing.T, e feature.Entitlements,
) (*switchableProvider, *errorRecorderProvider) {
	t.Helper()
	threaded := &switchableProvider{
		errorRecorderProvider: errorRecorderProvider{name: "slack"}}
	plain := &errorRecorderProvider{name: "webhook"}
	manager := NewManagerWithDependencies(Dependencies{
		Clock: clock.RealClock{}, Entitlements: e,
	})
	factory := func(
		name string, _ map[string]interface{}, _ transport.ProviderContext,
	) Provider {
		if name == "slack" {
			return threaded
		}
		return plain
	}
	err := initTestManager(manager, map[string]map[string]interface{}{
		"slack": {}, "webhook": {},
	}, nil, factory)
	if err != nil {
		t.Fatal(err)
	}
	return threaded, plain
}

func TestCommunityTurnsNativeThreadsOff(t *testing.T) {
	threaded, _ := initThreaded(t, feature.CommunityEntitlements())
	if !threaded.threadsOff {
		t.Fatal("Community must send every message unthreaded")
	}
}

func TestMissingEntitlementsTurnNativeThreadsOff(t *testing.T) {
	threaded, _ := initThreaded(t, nil)
	if !threaded.threadsOff {
		t.Fatal("nil entitlements are Community")
	}
}

func TestProKeepsNativeThreads(t *testing.T) {
	threaded, _ := initThreaded(t, feature.ProEntitlements())
	if threaded.threadsOff {
		t.Fatal("Pro keeps native threads")
	}
}

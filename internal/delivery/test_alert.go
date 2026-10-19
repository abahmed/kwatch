package delivery

import (
	"context"
	"fmt"
	"sync"
	"time"

	deliveryapi "github.com/abahmed/kwatch/internal/delivery/api"
)

// TestOutcome is what one provider did with a test alert.
type TestOutcome string

// Test alert outcomes.
const (
	TestSent    TestOutcome = "sent"
	TestFailed  TestOutcome = "failed"
	TestSkipped TestOutcome = "skipped"
)

// TestResult is one provider's answer to a test alert.
type TestResult struct {
	Provider string
	Outcome  TestOutcome
	Elapsed  time.Duration
	Err      error
	// Reason says why a provider was skipped.
	Reason string
}

// ProviderNames lists the configured providers in delivery order.
func (m *Manager) ProviderNames() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	generation := m.currentGenerationLocked()
	if generation == nil {
		return nil
	}
	return append([]string(nil), generation.order...)
}

func (m *Manager) testProviders(only string) []Provider {
	m.mu.Lock()
	defer m.mu.Unlock()
	generation := m.currentGenerationLocked()
	if generation == nil {
		return nil
	}
	var providers []Provider
	for _, name := range generation.order {
		if only == "" || name == only {
			providers = append(providers,
				generation.entries[name].provider)
		}
	}
	return providers
}

// SendTestAlert sends text straight to each provider, or only to the one
// named, in parallel with a per-provider timeout. It bypasses queues, the
// outbox, budgets and deduplication: nothing about it is recorded.
// Providers that drop plain messages (paging systems, issue trackers)
// would open something nothing closes, so they are skipped.
func (m *Manager) SendTestAlert(
	ctx context.Context, only, text string, timeout time.Duration,
) []TestResult {
	providers := m.testProviders(only)
	results := make([]TestResult, len(providers))
	var wg sync.WaitGroup
	for i, provider := range providers {
		results[i].Provider = provider.Name()
		if skipper, ok := provider.(deliveryapi.PlainMessageSkipper); ok &&
			skipper.SkipsPlainMessages() {
			results[i].Outcome = TestSkipped
			results[i].Reason = "paging or issue provider: a test " +
				"would open an alert nothing closes"
			continue
		}
		wg.Add(1)
		go func(slot *TestResult, provider Provider) {
			defer wg.Done()
			*slot = m.sendOneTest(ctx, provider, text, timeout)
		}(&results[i], provider)
	}
	wg.Wait()
	return results
}

func (m *Manager) sendOneTest(
	ctx context.Context, provider Provider, text string,
	timeout time.Duration,
) (result TestResult) {
	result.Provider = provider.Name()
	sendCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := m.nowTime()
	defer func() {
		if r := recover(); r != nil {
			result.Outcome = TestFailed
			result.Err = fmt.Errorf("provider panicked: %v", r)
		}
		result.Elapsed = m.nowTime().Sub(start)
	}()
	if err := provider.SendMessage(sendCtx, text); err != nil {
		result.Outcome, result.Err = TestFailed, err
		return result
	}
	result.Outcome = TestSent
	return result
}

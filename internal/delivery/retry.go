package delivery

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/binary"
	"errors"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/ratelimit"
)

const (
	// defaultRetryDelay is the first backoff when the configured delay is
	// missing or not positive.
	defaultRetryDelay = time.Second
	// maxBackoffShift is the largest attempt-1 that doubles the base delay.
	// A larger shift would overflow time.Duration, so the backoff is
	// capped at maxBackoff instead.
	maxBackoffShift = 30
)

type retryConfig struct {
	maxAttempts   int
	delay         time.Duration
	maxBackoff    time.Duration
	jitterEnabled bool
	jitterFactor  float64
}

func retryConfigFromRuntime(policy config.RetryPolicy) retryConfig {
	return normalizeRetryConfig(retryConfig{
		maxAttempts:   policy.MaxAttempts,
		delay:         policy.Delay,
		maxBackoff:    policy.MaxBackoff,
		jitterEnabled: policy.JitterEnabled,
		jitterFactor:  policy.JitterFactor,
	})
}

func normalizeRetryConfig(rc retryConfig) retryConfig {
	if rc.maxAttempts < 1 {
		rc.maxAttempts = 1
	}
	if rc.delay <= 0 {
		rc.delay = defaultRetryDelay
	}
	if rc.maxBackoff < 0 {
		rc.maxBackoff = defaultMaxBackoff
	}
	if rc.jitterFactor < 0 {
		rc.jitterFactor = 0
	}
	if rc.jitterFactor > 1 {
		rc.jitterFactor = 1
	}
	return rc
}

func backoffFor(
	attempt int,
	baseDelay, maxBackoff time.Duration,
) time.Duration {
	shift := attempt - 1
	if shift > maxBackoffShift {
		return maxBackoff
	}
	delay := baseDelay * time.Duration(1<<shift)
	if maxBackoff > 0 && (delay > maxBackoff || delay <= 0) {
		delay = maxBackoff
	}
	if delay < baseDelay {
		delay = baseDelay
	}
	return delay
}

func applyJitter(delay time.Duration, factor float64) time.Duration {
	if factor <= 0 {
		return delay
	}
	var seed [8]byte
	if _, err := cryptorand.Read(seed[:]); err != nil {
		return delay
	}
	ratio := float64(binary.LittleEndian.Uint64(seed[:])) /
		float64(^uint64(0))
	jitter := time.Duration(float64(delay) * factor * (ratio*2 - 1))
	return delay + jitter
}

func sendWithRetry(
	ctx context.Context,
	sendFn func() error,
	rc retryConfig,
	providerName string,
) error {
	rc = normalizeRetryConfig(rc)
	var lastErr error
	for attempt := 1; attempt <= rc.maxAttempts; attempt++ {
		err := sendFn()
		if err == nil {
			return nil
		}
		lastErr = err
		if transport.IsPermanent(err) {
			klog.ErrorS(
				loggedErr(err),
				"provider rejected the notification; not retrying",
				"provider",
				providerName,
			)
			return err
		}
		if isRateLimited(err) {
			// The provider named its wait. The caller blocks the whole
			// provider for it, without spending this job's attempts.
			return err
		}
		if attempt < rc.maxAttempts {
			err := waitBeforeRetry(ctx, err, attempt, rc, providerName)
			if err != nil {
				return err
			}
		}
	}
	klog.ErrorS(
		loggedErr(lastErr),
		"failed to deliver after retries",
		"provider",
		providerName,
		"maxAttempts",
		rc.maxAttempts,
	)
	return lastErr
}

// waitBeforeRetry counts the retry, picks the jittered backoff and
// sleeps. It returns ctx's error if the wait was cut short.
func waitBeforeRetry(
	ctx context.Context,
	sendErr error,
	attempt int,
	rc retryConfig,
	providerName string,
) error {
	metrics.DefaultRegistry().DeliveryRetries.Add(1)
	delay := retryDelay(attempt, rc)
	if rc.jitterEnabled {
		delay = applyJitter(delay, rc.jitterFactor)
		if delay <= 0 {
			delay = rc.delay
		}
	}
	klog.V(4).InfoS(
		"retrying provider delivery", "provider", providerName,
		"attempt", attempt, "maxAttempts", rc.maxAttempts,
		"backoff", delay, "error", loggedError(sendErr),
	)
	return sleepWithContext(ctx, delay)
}

func retryDelay(attempt int, rc retryConfig) time.Duration {
	if rc.maxBackoff > 0 {
		return backoffFor(attempt, rc.delay, rc.maxBackoff)
	}
	return rc.delay
}

// serverRetryAfter reports whether the provider rate-limited the request
// and the wait it asked for, capped. A zero wait means it named none.
func serverRetryAfter(err error) (time.Duration, bool) {
	var retryAfter *transport.RetryAfterError
	if errors.As(err, &retryAfter) {
		return capServerRetryAfter(retryAfter.RetryAfter), true
	}
	var rateLimit *ratelimit.Error
	if errors.As(err, &rateLimit) {
		return capServerRetryAfter(rateLimit.RetryAfter), true
	}
	return 0, false
}

// isRateLimited reports whether the provider asked kwatch to slow down.
func isRateLimited(err error) bool {
	_, ok := serverRetryAfter(err)
	return ok
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// maxServerRetryAfter bounds a provider-requested wait. The wait blocks
// the provider's queue, so an unbounded Retry-After (up to a day, or any
// HTTP date) would hold every notification behind it for that long.
const maxServerRetryAfter = maxProviderBackoff

func capServerRetryAfter(wait time.Duration) time.Duration {
	if wait > maxServerRetryAfter {
		return maxServerRetryAfter
	}
	return wait
}

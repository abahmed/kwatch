package delivery

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/binary"
	"errors"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/ratelimit"
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
		rc.delay = time.Second
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
	if shift > 30 {
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
		if err := sendFn(); err != nil {
			lastErr = err
			if event.IsPermanent(err) {
				klog.ErrorS(
					err,
					"provider rejected the notification; not retrying",
					"provider",
					providerName,
				)
				return err
			}
			if attempt < rc.maxAttempts {
				metrics.DefaultRegistry().DeliveryRetries.Add(1)
				delay, serverSpecified := retryDelay(err, attempt, rc)
				if rc.jitterEnabled && !serverSpecified {
					delay = applyJitter(delay, rc.jitterFactor)
					if delay <= 0 {
						delay = rc.delay
					}
				}
				klog.V(4).InfoS(
					"retrying provider delivery", "provider", providerName,
					"attempt", attempt, "maxAttempts", rc.maxAttempts,
					"backoff", delay,
				)
				if err := sleepWithContext(ctx, delay); err != nil {
					return err
				}
			}
			continue
		}
		return nil
	}
	klog.ErrorS(
		lastErr,
		"failed to deliver after retries",
		"provider",
		providerName,
		"maxAttempts",
		rc.maxAttempts,
	)
	return lastErr
}

func retryDelay(
	err error,
	attempt int,
	rc retryConfig,
) (time.Duration, bool) {
	delay := rc.delay
	if rc.maxBackoff > 0 {
		delay = backoffFor(attempt, rc.delay, rc.maxBackoff)
	}
	var retryAfter *event.RetryAfterError
	if errors.As(err, &retryAfter) && retryAfter.RetryAfter > 0 {
		return retryAfter.RetryAfter, true
	}
	var rateLimit *ratelimit.Error
	if errors.As(err, &rateLimit) && rateLimit.RetryAfter > 0 {
		return rateLimit.RetryAfter, true
	}
	return delay, false
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

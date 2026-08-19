package alert

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"text/template"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/ratelimit"
)

func extractRoutes(cfg map[string]interface{}) []config.AlertRoute {
	if r, ok := cfg["routes"]; ok {
		if routes, ok := r.([]interface{}); ok {
			out := make([]config.AlertRoute, 0, len(routes))
			for _, ri := range routes {
				if rm, ok := ri.(map[string]interface{}); ok {
					route := config.AlertRoute{}
					if ns, ok := rm["namespaces"]; ok {
						if arr, ok := ns.([]interface{}); ok {
							for _, n := range arr {
								route.Namespaces = append(route.Namespaces, fmt.Sprint(n))
							}
						}
					}
					if sev, ok := rm["severities"]; ok {
						if arr, ok := sev.([]interface{}); ok {
							for _, s := range arr {
								route.Severities = append(route.Severities, fmt.Sprint(s))
							}
						}
					}
					if rea, ok := rm["reasons"]; ok {
						if arr, ok := rea.([]interface{}); ok {
							for _, r := range arr {
								route.Reasons = append(route.Reasons, fmt.Sprint(r))
							}
						}
					}
					if len(route.Namespaces) > 0 || len(route.Severities) > 0 || len(route.Reasons) > 0 {
						out = append(out, route)
					}
				}
			}
			return out
		}
	}
	return nil
}

func extractTemplates(cfg map[string]interface{}) map[string]*template.Template {
	if raw, ok := cfg["templates"]; ok {
		if tpl, ok := raw.(map[string]interface{}); ok {
			out := make(map[string]*template.Template, len(tpl))
			for reason, rawBody := range tpl {
				if body, ok := rawBody.(string); ok {
					t, err := template.New(reason).Option("missingkey=zero").Parse(body)
					if err != nil {
						klog.ErrorS(err, "invalid provider template, skipping", "reason", reason)
						continue
					}
					out[strings.ToLower(reason)] = t
				}
			}
			return out
		}
	}
	return nil
}

type retryConfig struct {
	maxAttempts   int
	delay         time.Duration
	maxBackoff    time.Duration
	jitterEnabled bool
	jitterFactor  float64
}

func extractRetry(cfg map[string]interface{}) retryConfig {
	rc := retryConfig{
		maxAttempts:   3,
		delay:         time.Second,
		maxBackoff:    defaultMaxBackoff,
		jitterEnabled: false,
		jitterFactor:  0.25,
	}
	if r, ok := cfg["retry"]; ok {
		if rm, ok := r.(map[string]interface{}); ok {
			if a, ok := rm["maxAttempts"]; ok {
				n := 0
				switch v := a.(type) {
				case int:
					n = v
				case int64:
					n = int(v)
				case float64:
					n = int(v) // tolerate JSON/CRD paths
				}
				if n > 20 {
					n = 20
				}
				if n < 1 {
					n = 1
				}
				rc.maxAttempts = n
			}
			if d, ok := rm["delay"]; ok {
				if s, ok := d.(string); ok {
					if parsed, err := time.ParseDuration(s); err == nil {
						rc.delay = parsed
					}
				}
			}
			if b, ok := rm["maxBackoff"]; ok {
				if s, ok := b.(string); ok {
					if parsed, err := time.ParseDuration(s); err == nil {
						rc.maxBackoff = parsed
					}
				}
			}
			if j, ok := rm["jitterEnabled"]; ok {
				if b, ok := j.(bool); ok {
					rc.jitterEnabled = b
				}
			}
			if jf, ok := rm["jitterFactor"]; ok {
				switch v := jf.(type) {
				case float64:
					rc.jitterFactor = v
				case int:
					rc.jitterFactor = float64(v)
				}
				if rc.jitterFactor < 0 {
					rc.jitterFactor = 0
				}
				if rc.jitterFactor > 1 {
					rc.jitterFactor = 1
				}
			}
		}
	}
	return rc
}

// Init initializes AlertManager with provided config.
// Safe to call multiple times: shuts down existing workers before re-init.

func matchesRoute(route config.AlertRoute, inc *model.Incident) bool {
	if len(route.Namespaces) > 0 {
		found := false
		for _, ns := range route.Namespaces {
			if ns == inc.Namespace {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(route.Severities) > 0 {
		found := false
		for _, s := range route.Severities {
			if model.SeverityFromString(s) == inc.Severity {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(route.Reasons) > 0 {
		found := false
		for _, r := range route.Reasons {
			if r == inc.Reason {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// shouldDeliver checks whether an incident should be delivered to a provider.
// If the provider has no routes defined, all incidents are delivered.

func shouldDeliver(routes []config.AlertRoute, inc *model.Incident) bool {
	if len(routes) == 0 {
		return true
	}
	for _, route := range routes {
		if matchesRoute(route, inc) {
			return true
		}
	}
	return false
}

func backoffFor(attempt int, baseDelay, maxBackoff time.Duration) time.Duration {
	shift := attempt - 1
	if shift > 30 {
		return maxBackoff
	}
	d := baseDelay * time.Duration(1<<shift)
	if maxBackoff > 0 && (d > maxBackoff || d <= 0) {
		d = maxBackoff
	}
	if d < baseDelay {
		d = baseDelay
	}
	return d
}

func applyJitter(d time.Duration, factor float64) time.Duration {
	if factor <= 0 {
		return d
	}
	jitter := time.Duration(float64(d) * factor * (rand.Float64()*2 - 1))
	return d + jitter
}

func sendWithRetry(ctx context.Context, sendFn func() error, rc retryConfig, providerName string) error {
	var lastErr error
	for attempt := 1; attempt <= rc.maxAttempts; attempt++ {
		if err := sendFn(); err != nil {
			lastErr = err
			if attempt < rc.maxAttempts {
				sleepDur := rc.delay
				if rc.maxBackoff > 0 {
					sleepDur = backoffFor(attempt, rc.delay, rc.maxBackoff)
				}
				// When the server specifies a Retry-After, honor it exactly;
				// jitter must never shrink the wait below the server's request.
				serverSpecified := false
				var rae *event.RetryAfterError
				if errors.As(err, &rae) && rae.RetryAfter > 0 {
					sleepDur = rae.RetryAfter
					serverSpecified = true
				}
				var rle *ratelimit.Error
				if errors.As(err, &rle) && rle.RetryAfter > 0 {
					sleepDur = rle.RetryAfter
					serverSpecified = true
				}
				if rc.jitterEnabled && !serverSpecified {
					sleepDur = applyJitter(sleepDur, rc.jitterFactor)
					if sleepDur <= 0 {
						sleepDur = rc.delay
					}
				}
				klog.V(4).InfoS("retrying provider delivery",
					"provider", providerName,
					"attempt", attempt,
					"maxAttempts", rc.maxAttempts,
					"backoff", sleepDur)
				if err := sleepWithContext(ctx, sleepDur); err != nil {
					return err
				}
			}
			continue
		}
		return nil
	}
	klog.ErrorS(lastErr, "failed to deliver after retries",
		"provider", providerName,
		"maxAttempts", rc.maxAttempts)
	return lastErr
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// VerifyAll runs credential pre-flight on all providers that support it.
// Returns a map of provider name → error (nil = verified OK).

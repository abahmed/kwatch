package license

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// serverTime prefers the body's srv_time (unix seconds), then the
// X-Kwatch-Srv-Time header, then the HTTP Date header.
func serverTime(srv int64, h http.Header) time.Time {
	if srv <= 0 {
		srv, _ = strconv.ParseInt(
			strings.TrimSpace(h.Get(srvTimeHeader)), 10, 64)
	}
	if srv > 0 {
		return time.Unix(srv, 0).UTC()
	}
	if t, err := http.ParseTime(h.Get("Date")); err == nil {
		return t.UTC()
	}
	return time.Time{}
}

func classify(status int, w wireResponse, retryAfter string) error {
	switch {
	case status >= 500:
		return &TransientError{Status: status, Err: fmt.Errorf("status %d", status)}
	case status == http.StatusTooManyRequests && w.Error == "transfer_limit":
		return ErrTransferLimit{
			RetryAfter: time.Duration(max(w.RetryAfter, 0)) * time.Second,
		}
	case status == http.StatusTooManyRequests:
		return ErrRateLimited{RetryAfter: parseRetryAfter(retryAfter)}
	case status == http.StatusUnauthorized:
		return ErrUnauthorized
	}
	switch w.Error {
	case "bound_to_other_cluster":
		var since time.Time
		if w.BoundSince > 0 {
			since = time.Unix(w.BoundSince, 0).UTC()
		}
		return ErrConflict{BoundSince: since}
	case "instance_superseded":
		return ErrInstanceSuperseded
	case "revoked":
		return ErrRevoked
	case "suspended":
		return ErrSuspended
	case "expired":
		return ErrServerExpired
	case "trial_already_used":
		return ErrTrialUsed
	case "key_replaced":
		return ErrKeyReplaced
	case "cluster_proof_mismatch":
		return ErrProofMismatch
	case "stale_timestamp":
		return ErrStaleTimestamp
	case "nonce_reused":
		return ErrNonceReused
	}
	return &StatusError{Status: status, Code: w.Error}
}

// parseRetryAfter accepts delta-seconds or an HTTP date; unparseable values
// yield a minute.
func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	d := time.Minute
	if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
		d = time.Duration(secs) * time.Second
	} else if t, err := http.ParseTime(v); err == nil {
		d = time.Until(t)
	}
	if d < 0 {
		d = 0
	}
	return min(d, maxRetryAfter)
}

package license

import (
	"errors"
	"strings"

	"k8s.io/klog/v2"
)

// syncOK is the synced outcome of a sync the server accepted.
const syncOK = "ok"

// logSync reports the outcome of a server sync. The first success, and
// the first success after a failure, is logged at the default level with
// the masked license id, plan and expiry; a routine renewal only at
// -v=2. A failure is a warning once per distinct reason. The key is
// never logged. A conflict has its own lines (manager_conflict.go).
func (m *Manager) logSync(b Binding, err error) {
	var conflict ErrConflict
	if errors.As(err, &conflict) {
		return
	}
	m.mu.Lock()
	previous, lease := m.synced, m.lease
	outcome := syncOK
	if err != nil {
		outcome = m.redact(err.Error())
	}
	m.synced = outcome
	m.mu.Unlock()
	if err != nil {
		if outcome != previous {
			klog.Warningf("license check with the license server "+
				"failed: component=license lid=%s reason=%q",
				MaskLID(b.LID), outcome)
		}
		return
	}
	fields := []any{"component", "license", "operation", "sync",
		"lid", MaskLID(b.LID)}
	if l, verr := m.v.VerifyLease(lease, b, m.effective()); verr == nil {
		fields = append(fields, "plan", planName(l.Plan),
			"licenseExpires", l.LicenseEnd(),
			"leaseExpires", l.ExpiresAt())
	}
	if previous == syncOK {
		klog.V(2).InfoS("license lease renewed", fields...)
		return
	}
	klog.InfoS("license confirmed by the license server", fields...)
}

// redact removes the license key from text that may echo a request.
func (m *Manager) redact(text string) string {
	if m.cfg.Key == "" {
		return text
	}
	return strings.ReplaceAll(text, m.cfg.Key, "[redacted]")
}

// planName is how a plan is logged: every plan a key carries, the
// legacy "trial" value included, is Pro to the operator.
func planName(plan string) string {
	if plan == "" {
		return ""
	}
	return "Pro"
}

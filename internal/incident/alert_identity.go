package incident

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// SetAlertIdentity sets what tells this cluster's alert keys apart from
// another cluster's when no cluster name is configured (see
// Config.AlertIdentity). It is called once, before the first Apply.
func (m *Manager) SetAlertIdentity(identity string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg.AlertIdentity = identity
}

// newAlertKey is the alert key a new incident gets. With an alert
// identity the identity is hashed into it, so the same failure in two
// unnamed clusters gets two keys. An incident keeps the key it was
// announced with, so alerts that were open before the identity was
// known are still updated and resolved under their old key.
func (c Config) newAlertKey(
	root inventory.EntityID, mode detection.Mode,
) string {
	if c.AlertIdentity == "" {
		return alertKey(root, mode)
	}
	sum := sha256.Sum256([]byte(
		c.AlertIdentity + "|" + root.String() + "|" + string(mode)))
	return "alert-" + hex.EncodeToString(sum[:hashBytes])
}

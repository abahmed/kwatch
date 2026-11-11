package dashboard

import (
	"encoding/json"
	"strings"
	"testing"
)

// The banner's advice follows the health reason (pages.js
// providerFix), so /api/status carries it.
func TestChannelHealthCarriesItsReason(t *testing.T) {
	b, err := json.Marshal(ChannelHealth{Channel: "slack", Failing: true,
		Reason: "provider_unavailable"})
	if err != nil || !strings.Contains(string(b),
		`"reason":"provider_unavailable"`) {
		t.Fatalf("%s %v", b, err)
	}
}

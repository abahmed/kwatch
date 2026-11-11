package delivery

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/transport"
)

func rejection(code int) error {
	return transport.Permanent(&transport.StatusError{
		Provider: "p", StatusCode: code, Body: "bad"})
}

func healthReason(m *Manager, entry *providerEntry) string {
	m.healthMu.Lock()
	defer m.healthMu.Unlock()
	return m.providerReasons[entry.lookupName()]
}

// One payload refused with a 400 is not a provider that "may have an
// expired webhook"; the provider is not marked rejecting.
func TestSingleBadRequestDoesNotMarkProviderRejected(t *testing.T) {
	m, entry := fallbackPair(&errorRecorderProvider{name: "P"},
		&errorRecorderProvider{name: "Fallback"})

	m.recordProviderFailure(entry, rejection(400))

	assert.Empty(t, healthReason(m, entry))
}

// Access errors mark the provider at once.
func TestAccessErrorsMarkProviderRejectedAtOnce(t *testing.T) {
	for _, code := range []int{401, 403, 404, 410} {
		m, entry := fallbackPair(&errorRecorderProvider{name: "P"},
			&errorRecorderProvider{name: "Fallback"})

		m.recordProviderFailure(entry, rejection(code))

		assert.Equal(t, reasonRejected, healthReason(m, entry), code)
	}
}

// Several rejections in a row mark it; a success starts the count over.
func TestRepeatedRejectionsMarkProviderRejected(t *testing.T) {
	m, entry := fallbackPair(&errorRecorderProvider{name: "P"},
		&errorRecorderProvider{name: "Fallback"})

	m.recordProviderFailure(entry, rejection(400))
	m.recordProviderFailure(entry, rejection(400))
	m.recordProviderSuccess(entry)
	m.recordProviderFailure(entry, rejection(400))
	assert.Empty(t, healthReason(m, entry))

	m.recordProviderFailure(entry, rejection(422))
	m.recordProviderFailure(entry, rejection(400))
	assert.Equal(t, reasonRejected, healthReason(m, entry))
}

// A refused credential reported without a status still marks it.
func TestRefusedCredentialWithoutStatusMarksProviderRejected(t *testing.T) {
	m, entry := fallbackPair(&errorRecorderProvider{name: "P"},
		&errorRecorderProvider{name: "Fallback"})

	m.recordProviderFailure(entry,
		transport.Permanent(errors.New("slack: invalid_auth")))

	assert.Equal(t, reasonRejected, healthReason(m, entry))
}

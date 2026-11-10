package email

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

func dataEndClient(t *testing.T, f *fakeSMTP) *Email {
	t.Helper()
	port := f.start(t)
	c := NewEmail(relayConfig(port, "k@example.test", "a@example.test"),
		"dev", transport.Dependencies{})
	require.NotNil(t, c)
	c.smtpConfig.idleTimeout = 150 * time.Millisecond
	return c
}

// A server that scans the message answers the end of DATA later than the
// idle limit; the mail is accepted and must not be cut off and retried.
func TestEmailSlowEndOfDataReplyIsWaitedFor(t *testing.T) {
	f := &fakeSMTP{endDelay: 600 * time.Millisecond}
	c := dataEndClient(t, f)
	c.smtpConfig.dataEndTimeout = 5 * time.Second

	require.NoError(t, c.SendIncident(t.Context(), providertest.Announce()))
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.True(t, f.accepted)
}

// No answer to the end of DATA even after the long wait: the mail may
// have been accepted, so the failure is permanent and is not retried.
func TestEmailNoAnswerAfterBodyIsNotRetried(t *testing.T) {
	f := &fakeSMTP{endDelay: 3 * time.Second}
	c := dataEndClient(t, f)
	c.smtpConfig.dataEndTimeout = 300 * time.Millisecond

	err := c.SendIncident(t.Context(), providertest.Announce())
	require.Error(t, err)
	assert.True(t, transport.IsPermanent(err), err)
	assert.Contains(t, err.Error(), "may have been delivered")
}

func TestEmailSessionLimitCoversTheEndOfDataWait(t *testing.T) {
	c := smtpConfig{sessionTimeout: time.Second}
	assert.GreaterOrEqual(t, c.session(), c.dataEnd()+c.idle())
	assert.GreaterOrEqual(t, smtpConfig{}.session(),
		smtpDataEndTimeout+smtpTimeout)
}

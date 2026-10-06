package delivery

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

// failingThenOKProvider fails its first failures sends, then succeeds.
type failingThenOKProvider struct {
	failures int
	calls    int
	err      error
}

func (p *failingThenOKProvider) Name() string { return "scripted" }

func (p *failingThenOKProvider) SendMessage(context.Context, string) error {
	return p.next()
}

func (p *failingThenOKProvider) SendIncident(
	context.Context, notification.Message,
) error {
	return p.next()
}

func (p *failingThenOKProvider) next() error {
	p.calls++
	if p.calls <= p.failures {
		return p.err
	}
	return nil
}

// captureKlog returns what klog writes until the test ends.
func captureKlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var out bytes.Buffer
	klog.LogToStderr(false)
	klog.SetOutput(&out)
	t.Cleanup(func() {
		klog.Flush()
		klog.SetOutput(nil)
		klog.LogToStderr(true)
	})
	return &out
}

// sendLine is the provider send line of captured log output.
func sendLine(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, `"provider send"`) {
			return line
		}
	}
	return ""
}

func dispatchOnce(
	p *failingThenOKProvider, job deliverJob, attempts int,
) error {
	manager := newTestManager()
	entry := &providerEntry{provider: p}
	opts := deliverOpts{retry: retryConfig{maxAttempts: attempts}}
	return manager.dispatch(context.Background(), entry, job, opts)
}

func TestDispatchLogsSuccessfulSendWithItsFields(t *testing.T) {
	out := captureKlog(t)
	job := deliverJob{kind: jobIncident, incident: &notification.Message{
		Key: "deploy/shop/api", Revision: 1, Note: "secret body text"}}

	assert.NoError(t, dispatchOnce(&failingThenOKProvider{}, job, 1))
	klog.Flush()

	line := sendLine(out.String())
	assert.Contains(t, line, `"provider send"`)
	assert.Contains(t, line, `provider="scripted"`)
	assert.Contains(t, line, `key="deploy/shop/api"`)
	assert.Contains(t, line, `kind="incident"`)
	assert.Contains(t, line, `placement="root"`)
	assert.Contains(t, line, `result="ok"`)
	assert.Contains(t, line, "retries=0")
	assert.NotContains(t, line, "secret body text")
}

func TestDispatchLogsFailureWithRetriesAndNoURL(t *testing.T) {
	out := captureKlog(t)
	failure := errors.New(
		"post https://hooks.example.com/services/T0/B0/token: refused")
	provider := &failingThenOKProvider{failures: 5, err: failure}
	job := deliverJob{kind: jobIncident, incident: &notification.Message{
		Key: "k", Revision: 3, Status: notification.StatusResolved}}

	assert.Error(t, dispatchOnce(provider, job, 2))
	klog.Flush()

	line := sendLine(out.String())
	assert.Contains(t, line, `kind="resolve"`)
	assert.Contains(t, line, `placement="edit"`)
	assert.Contains(t, line, `result="error"`)
	assert.Contains(t, line, "retries=1")
	assert.Contains(t, line, "refused")
	assert.NotContains(t, line, "hooks.example.com")
	assert.NotContains(t, line, "token")
}

func TestDispatchLogsThreadReplyAndPlainMessage(t *testing.T) {
	out := captureKlog(t)
	update := deliverJob{kind: jobIncident,
		incident: &notification.Message{Key: "k", Revision: 2}}
	plain := deliverJob{kind: jobMessage, msg: "hello"}

	assert.NoError(t, dispatchOnce(&failingThenOKProvider{}, update, 1))
	assert.NoError(t, dispatchOnce(&failingThenOKProvider{}, plain, 1))
	klog.Flush()

	assert.Contains(t, out.String(), `placement="thread"`)
	assert.Contains(t, out.String(), `key="message"`)
}

func TestLoggedErrorCutsOnACharacterBoundary(t *testing.T) {
	text := strings.Repeat("é", maxLoggedError)
	got := loggedError(errors.New(text))
	assert.True(t, utf8.ValidString(got), "cut mid-character")
	assert.True(t, strings.HasSuffix(got, "..."))
	assert.LessOrEqual(t, len(got), maxLoggedError+3)
}

// Every delivery log line scrubs the URLs of an error, not only the send
// log: a provider SDK error can quote a webhook URL.
func TestDeliveryLogsNeverCarryProviderURLs(t *testing.T) {
	var buf bytes.Buffer
	klog.LogToStderr(false)
	klog.SetOutput(&buf)
	defer klog.LogToStderr(true)
	secret := errors.New(
		"Post https://hooks.example.com/services/T0/B0/s3cret: refused")

	_ = sendWithRetry(context.Background(),
		func() error { return transport.Permanent(secret) },
		retryConfig{maxAttempts: 1}, "slack")
	_ = sendWithRetry(context.Background(),
		func() error { return secret },
		retryConfig{maxAttempts: 2, delay: time.Millisecond}, "slack")
	klog.Flush()

	assert.NotEmpty(t, buf.String())
	assert.NotContains(t, buf.String(), "s3cret")
}

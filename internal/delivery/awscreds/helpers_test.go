package awscreds

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"k8s.io/klog/v2"
)

// Secret-looking values the tests hand out; none may reach a log line
// or an error.
const (
	testWebToken  = "eyJweb-identity-token-value"
	testPodToken  = "pod-identity-agent-token-value"
	testSecretKey = "tmp-secret-access-key-value"
	testSession   = "tmp-session-token-value"
)

var testStart = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// fakeClock is a clock the test moves.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{now: testStart} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// writeToken writes a token file in the test's directory.
func writeToken(t *testing.T, path, token string) string {
	t.Helper()
	if path == "" {
		path = filepath.Join(t.TempDir(), "token")
	}
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
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

// assertNoSecrets fails when text carries any test secret.
func assertNoSecrets(t *testing.T, what, text string) {
	t.Helper()
	for _, secret := range []string{testWebToken, testPodToken,
		testSecretKey, testSession} {
		if bytes.Contains([]byte(text), []byte(secret)) {
			t.Fatalf("%s carries a secret: %s", what, text)
		}
	}
}

func klogFlush() { klog.Flush() }

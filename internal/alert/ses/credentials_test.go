package ses

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/awscreds"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

var credsNow = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func sesSettings(extra map[string]interface{}) map[string]interface{} {
	m := map[string]interface{}{"from": "kwatch@example.com",
		"to": "ops@example.com"}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func TestSesSignsWithPodIdentityCredentials(t *testing.T) {
	var agentAuth, sesAuth string
	agent := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			agentAuth = r.Header.Get("Authorization")
			_, _ = fmt.Fprintf(w, `{"AccessKeyId":"ASIAPOD",`+
				`"SecretAccessKey":"s","Token":"t","Expiration":%q}`,
				credsNow.Add(time.Hour).Format(time.RFC3339))
		}))
	defer agent.Close()
	api := httptest.NewServer(http.HandlerFunc(
		func(_ http.ResponseWriter, r *http.Request) {
			sesAuth = r.Header.Get("Authorization")
		}))
	defer api.Close()
	token := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(token, []byte("agent-jwt"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(awscreds.EnvAgentURL, agent.URL+"/v1/credentials")
	t.Setenv(awscreds.EnvAgentToken, token)
	c := NewSes(sesSettings(map[string]interface{}{
		"endpoint": api.URL + "/"}), "dev", transport.Dependencies{
		HTTPClient: api.Client(),
		Clock:      clock.Func(func() time.Time { return credsNow }),
	})
	if c == nil {
		t.Fatal("ses with Pod Identity was not created")
	}
	if err := c.SendMessage(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if agentAuth != "agent-jwt" ||
		!strings.Contains(sesAuth, "Credential=ASIAPOD/") {
		t.Fatalf("agent auth %q, ses auth %q", agentAuth, sesAuth)
	}
}

func TestSesStaticExpiredTokenIsPermanentWithItsReason(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`<ErrorResponse><Error><Code>` +
				`ExpiredToken</Code></Error></ErrorResponse>`))
		}))
	defer api.Close()
	c := NewSes(sesSettings(map[string]interface{}{
		"accessKeyId": "ASIAX", "secretAccessKey": "s",
		"sessionToken": "t", "endpoint": api.URL}), "dev",
		transport.Dependencies{HTTPClient: api.Client(),
			Clock: clock.Func(func() time.Time { return credsNow })})
	err := c.SendMessage(context.Background(), "hi")
	if !transport.IsPermanent(err) ||
		!errors.Is(err, awscreds.ErrSessionTokenExpired) {
		t.Fatalf("err = %v", err)
	}
}

func TestSesEndpointOverride(t *testing.T) {
	keys := map[string]interface{}{"accessKeyId": "a",
		"secretAccessKey": "s"}
	with := func(k, v string) map[string]interface{} {
		m := sesSettings(keys)
		m[k] = v
		return m
	}
	c := NewSes(with("endpoint", "https://ses.vpce.example.com/"), "dev",
		testDeps)
	if c.url != "https://ses.vpce.example.com/" {
		t.Fatalf("url = %s", c.url)
	}
	if NewSes(with("endpoint", "ftp://x"), "dev", testDeps) != nil {
		t.Fatal("an invalid endpoint was accepted")
	}
	for _, name := range []string{awscreds.EnvRoleARN,
		awscreds.EnvAgentURL} {
		t.Setenv(name, "")
	}
	if NewSes(sesSettings(nil), "dev", testDeps) != nil {
		t.Fatal("ses without credentials was created")
	}
}

// SES is not offered in the China regions: such a provider does not
// start and the log says why, unless an endpoint override names where
// to send.
func TestSesRefusesAChinaRegionWithoutAnEndpoint(t *testing.T) {
	config := sesSettings(map[string]interface{}{"accessKeyId": "a",
		"secretAccessKey": "s", "region": "cn-northwest-1"})
	_, err := newSes(config, "cn-northwest-1", testDeps)
	if err == nil || !strings.Contains(err.Error(),
		"SES is not offered in the China region cn-northwest-1") {
		t.Fatalf("err = %v", err)
	}
	if NewSes(config, "dev", testDeps) != nil {
		t.Fatal("ses started in a China region")
	}
	config["endpoint"] = "https://ses.vpce.example.com/"
	if c := NewSes(config, "dev", testDeps); c == nil ||
		c.url != "https://ses.vpce.example.com/" {
		t.Fatal("an endpoint override was refused")
	}
}

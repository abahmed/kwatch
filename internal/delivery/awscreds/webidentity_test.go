package awscreds

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

const stsAnswerXML = `<AssumeRoleWithWebIdentityResponse ` +
	`xmlns="https://sts.amazonaws.com/doc/2011-06-15/">` +
	`<AssumeRoleWithWebIdentityResult><Credentials>` +
	`<AccessKeyId>%s</AccessKeyId>` +
	`<SecretAccessKey>` + testSecretKey + `</SecretAccessKey>` +
	`<SessionToken>` + testSession + `</SessionToken>` +
	`<Expiration>%s</Expiration>` +
	`</Credentials></AssumeRoleWithWebIdentityResult>` +
	`</AssumeRoleWithWebIdentityResponse>`

// fakeSTS answers AssumeRoleWithWebIdentity with credentials valid for
// ttl and records each request's form.
type fakeSTS struct {
	clock *fakeClock
	ttl   time.Duration
	mu    sync.Mutex
	forms []url.Values
	auth  []string
}

func (f *fakeSTS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	form, _ := url.ParseQuery(string(body))
	f.mu.Lock()
	f.forms = append(f.forms, form)
	f.auth = append(f.auth, r.Header.Get("Authorization"))
	n := len(f.forms)
	f.mu.Unlock()
	expires := f.clock.Now().Add(f.ttl).Format(time.RFC3339)
	_, _ = fmt.Fprintf(w, stsAnswerXML, fmt.Sprintf("ASIA%d", n), expires)
}

func newWebIdentity(
	t *testing.T, srv *httptest.Server, clk clock.Clock, file string,
) Source {
	t.Helper()
	src, err := NewWebIdentity(WebIdentityConfig{
		Provider: "SNS", RoleARN: "arn:aws:iam::1:role/kwatch",
		TokenFile: file, Endpoint: srv.URL + "/", Now: clk.Now,
		Sender: transport.NewWithDependencies(transport.Dependencies{
			HTTPClient: srv.Client(), Clock: clk}),
	})
	if err != nil {
		t.Fatal(err)
	}
	return src
}

func TestWebIdentityRefreshesAndRereadsTheTokenFile(t *testing.T) {
	logs := captureKlog(t)
	clk := newFakeClock()
	sts := &fakeSTS{clock: clk, ttl: 10 * time.Minute}
	srv := httptest.NewTLSServer(sts)
	defer srv.Close()
	file := writeToken(t, "", testWebToken+"-1")
	src := newWebIdentity(t, srv, clk, file)

	first, err := src.Credentials(context.Background())
	if err != nil || first.AccessKeyID != "ASIA1" ||
		first.SecretAccessKey != testSecretKey ||
		first.SessionToken != testSession {
		t.Fatalf("credentials = %+v, %v", first, err)
	}
	writeToken(t, file, testWebToken+"-2")
	clk.advance(4 * time.Minute)
	if got := keyOf(t, src); got != "ASIA1" {
		t.Fatalf("fetched again too early: %s", got)
	}
	clk.advance(2 * time.Minute) // 4 minutes before expiry
	if got := keyOf(t, src); got != "ASIA2" {
		t.Fatalf("not refreshed before expiry: %s", got)
	}
	if len(sts.forms) != 2 {
		t.Fatalf("STS calls = %d, want 2", len(sts.forms))
	}
	want := map[string]string{
		"Action": "AssumeRoleWithWebIdentity", "Version": "2011-06-15",
		"RoleArn":         "arn:aws:iam::1:role/kwatch",
		"RoleSessionName": "kwatch", "WebIdentityToken": testWebToken + "-1",
	}
	for key, value := range want {
		if got := sts.forms[0].Get(key); got != value {
			t.Errorf("%s = %q, want %q", key, got, value)
		}
	}
	if got := sts.forms[1].Get("WebIdentityToken"); got !=
		testWebToken+"-2" {
		t.Errorf("rotated token file not re-read: %q", got)
	}
	if sts.auth[0] != "" {
		t.Errorf("STS request was signed: %q", sts.auth[0])
	}
	klogFlush()
	assertNoSecrets(t, "log", logs.String())
	if !strings.Contains(logs.String(), "arn:aws:iam::1:role/kwatch") {
		t.Errorf("role ARN not logged: %s", logs.String())
	}
}

func TestWebIdentityErrorCarriesTheCodeButNoSecret(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintf(w, `<ErrorResponse><Error>`+
				`<Code>InvalidIdentityToken</Code><Message>%s</Message>`+
				`</Error></ErrorResponse>`, html.EscapeString(string(body)))
		}))
	defer srv.Close()
	clk := newFakeClock()
	src := newWebIdentity(t, srv, clk, writeToken(t, "", testWebToken))
	_, err := src.Credentials(context.Background())
	if err == nil || !strings.Contains(err.Error(), "InvalidIdentityToken") {
		t.Fatalf("err = %v, want the STS code", err)
	}
	assertNoSecrets(t, "error", err.Error())
}

func TestWebIdentityNeedsHTTPS(t *testing.T) {
	for _, endpoint := range []string{"http://sts.example.com/", "sts"} {
		_, err := NewWebIdentity(WebIdentityConfig{RoleARN: "arn",
			TokenFile: "/t", Endpoint: endpoint})
		if err == nil {
			t.Errorf("endpoint %q accepted", endpoint)
		}
	}
}

package awscreds

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPodIdentitySendsTheTokenFileAsAuthorization(t *testing.T) {
	logs := captureKlog(t)
	clk := newFakeClock()
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			seen = append(seen, r.Header.Get("Authorization"))
			if r.Method != http.MethodGet || r.URL.Path != "/v1/credentials" {
				http.NotFound(w, r)
				return
			}
			_, _ = fmt.Fprintf(w, `{"AccessKeyId":"ASIAPOD",`+
				`"SecretAccessKey":%q,"Token":%q,"Expiration":%q}`,
				testSecretKey, testSession,
				clk.Now().Add(time.Hour).Format(time.RFC3339))
		}))
	defer srv.Close()
	file := writeToken(t, "", testPodToken)
	src, err := NewPodIdentity(PodIdentityConfig{Provider: "SES",
		URL: srv.URL + "/v1/credentials", TokenFile: file, Now: clk.Now})
	if err != nil {
		t.Fatal(err)
	}
	creds, err := src.Credentials(context.Background())
	if err != nil || creds.AccessKeyID != "ASIAPOD" ||
		creds.SessionToken != testSession {
		t.Fatalf("credentials = %+v, %v", creds, err)
	}
	writeToken(t, file, testPodToken+"-2")
	src.Invalidate()
	keyOf(t, src)
	if len(seen) != 2 || seen[0] != testPodToken ||
		seen[1] != testPodToken+"-2" {
		t.Fatalf("Authorization headers = %q", seen)
	}
	klogFlush()
	assertNoSecrets(t, "log", logs.String())
}

func TestPodIdentityFailureCarriesNoSecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(r.Header.Get("Authorization")))
		}))
	defer srv.Close()
	src, err := NewPodIdentity(PodIdentityConfig{URL: srv.URL,
		TokenFile: writeToken(t, "", testPodToken),
		Now:       newFakeClock().Now})
	if err != nil {
		t.Fatal(err)
	}
	_, err = src.Credentials(context.Background())
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("err = %v", err)
	}
	assertNoSecrets(t, "error", err.Error())
}

func TestPodIdentityRefusesOtherHosts(t *testing.T) {
	_, err := NewPodIdentity(PodIdentityConfig{
		URL:       "http://169.254.169.254/latest/meta-data/",
		TokenFile: "/t", Now: newFakeClock().Now})
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("err = %v, want the host refused", err)
	}
}

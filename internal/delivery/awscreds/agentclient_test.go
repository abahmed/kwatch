package awscreds

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckAgentURL(t *testing.T) {
	allowed := []string{
		"http://169.254.170.23/v1/credentials",
		"http://[fd00:ec2::23]/v1/credentials",
		"http://127.0.0.1:8080/creds", "http://[::1]/creds",
		"http://localhost/creds",
	}
	for _, raw := range allowed {
		if err := CheckAgentURL(raw); err != nil {
			t.Errorf("%s refused: %v", raw, err)
		}
	}
	refused := []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://169.254.170.2/v2/credentials", "http://10.0.0.1/",
		"http://agent.example.com/", "http://[fd00:ec2::254]/",
		"ftp://127.0.0.1/", "not a url", "",
	}
	for _, raw := range refused {
		if err := CheckAgentURL(raw); err == nil {
			t.Errorf("%s accepted", raw)
		}
	}
}

func TestAgentControlChecksTheDialedAddress(t *testing.T) {
	for address, ok := range map[string]bool{
		"169.254.170.23:80": true, "[fd00:ec2::23]:80": true,
		"127.0.0.1:1": true, "[::1]:1": true,
		"169.254.169.254:80": false, "10.1.2.3:80": false,
		"[fd00:ec2::254]:80": false, "8.8.8.8:443": false,
	} {
		err := agentControl(context.Background(), "tcp", address, nil)
		if (err == nil) != ok {
			t.Errorf("%s: err = %v, want allowed %v", address, err, ok)
		}
	}
}

func TestAgentClientNeverFollowsARedirect(t *testing.T) {
	followed := false
	target := httptest.NewServer(http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) { followed = true }))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL, http.StatusFound)
		}))
	defer srv.Close()
	agent := podAgent{config: PodIdentityConfig{URL: srv.URL,
		TokenFile: writeToken(t, "", testPodToken)},
		client: newAgentClient()}
	if _, _, err := agent.fetch(context.Background()); err == nil {
		t.Fatal("a redirect was accepted")
	}
	if followed {
		t.Fatal("the redirect was followed with the agent token")
	}
}

func TestAgentClientIgnoresTheProxy(t *testing.T) {
	transport := newAgentClient().Transport.(*http.Transport)
	if transport.Proxy != nil {
		t.Fatal("the agent client uses a proxy")
	}
}

func TestAgentClientRefusesOtherAddressesAtDialTime(t *testing.T) {
	// 192.0.2.1 is TEST-NET-1: the dial is refused before connecting.
	agent := podAgent{config: PodIdentityConfig{
		URL:       "http://192.0.2.1:9/v1/credentials",
		TokenFile: writeToken(t, "", testPodToken)},
		client: newAgentClient()}
	_, _, err := agent.fetch(context.Background())
	if !errors.Is(err, errAgentAddress) {
		t.Fatalf("err = %v, want the address refused", err)
	}
}

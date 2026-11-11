package sns

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/awscreds"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

var credsNow = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// fakeAWS is an STS and an SNS endpoint: STS hands out key ASIA<n>,
// SNS answers each publish with the next status in sns.
type fakeAWS struct {
	mu       sync.Mutex
	stsCalls int
	statuses []int
	keys     []string
}

func (f *fakeAWS) sts(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	f.stsCalls++
	n := f.stsCalls
	f.mu.Unlock()
	_, _ = fmt.Fprintf(w, `<AssumeRoleWithWebIdentityResponse>`+
		`<AssumeRoleWithWebIdentityResult><Credentials>`+
		`<AccessKeyId>ASIA%d</AccessKeyId>`+
		`<SecretAccessKey>s</SecretAccessKey>`+
		`<SessionToken>t%d</SessionToken>`+
		`<Expiration>%s</Expiration></Credentials>`+
		`</AssumeRoleWithWebIdentityResult>`+
		`</AssumeRoleWithWebIdentityResponse>`, n, n,
		credsNow.Add(time.Hour).Format(time.RFC3339))
}

func (f *fakeAWS) sns(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	auth := r.Header.Get("Authorization")
	f.keys = append(f.keys, strings.SplitN(
		strings.TrimPrefix(auth, "AWS4-HMAC-SHA256 Credential="),
		"/", 2)[0])
	status := http.StatusOK
	if len(f.statuses) > 0 {
		status, f.statuses = f.statuses[0], f.statuses[1:]
	}
	w.WriteHeader(status)
	if status == http.StatusForbidden {
		_, _ = w.Write([]byte(`<ErrorResponse><Error><Code>` +
			`ExpiredToken</Code></Error></ErrorResponse>`))
	}
}

// webIdentitySns is an Sns using IRSA against fake STS and SNS servers.
func webIdentitySns(t *testing.T, aws *fakeAWS) *Sns {
	t.Helper()
	sts := httptest.NewTLSServer(http.HandlerFunc(aws.sts))
	t.Cleanup(sts.Close)
	api := httptest.NewServer(http.HandlerFunc(aws.sns))
	t.Cleanup(api.Close)
	token := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(token, []byte("jwt"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(awscreds.EnvRoleARN, "arn:aws:iam::1:role/kwatch")
	t.Setenv(awscreds.EnvTokenFile, token)
	t.Setenv(awscreds.EnvSTSEndpoint, sts.URL+"/")
	c := NewSns(map[string]interface{}{"topicArn": topic,
		"endpoint": api.URL + "/"}, "dev", transport.Dependencies{
		HTTPClient: sts.Client(),
		Clock:      clock.Func(func() time.Time { return credsNow }),
	})
	if c == nil {
		t.Fatal("sns with web identity was not created")
	}
	return c
}

func TestSnsSignsWithWebIdentityCredentials(t *testing.T) {
	aws := &fakeAWS{}
	c := webIdentitySns(t, aws)
	for i := 0; i < 2; i++ {
		if err := c.SendMessage(context.Background(), "hi"); err != nil {
			t.Fatal(err)
		}
	}
	if aws.stsCalls != 1 || strings.Join(aws.keys, ",") !=
		"ASIA1,ASIA1" {
		t.Fatalf("sts calls %d, keys %v", aws.stsCalls, aws.keys)
	}
}

func TestSnsExpiredTokenFetchesNewCredentialsOnce(t *testing.T) {
	aws := &fakeAWS{statuses: []int{403, 403, 403}}
	c := webIdentitySns(t, aws)
	err := c.SendMessage(context.Background(), "hi")
	if err == nil || transport.IsPermanent(err) ||
		!errors.Is(err, awscreds.ErrCredentialsRefused) {
		t.Fatalf("first refusal = %v, want retryable", err)
	}
	err = c.SendMessage(context.Background(), "hi")
	if !transport.IsPermanent(err) {
		t.Fatalf("second refusal = %v, want permanent", err)
	}
	if aws.stsCalls != 2 || strings.Join(aws.keys, ",") !=
		"ASIA1,ASIA2" {
		t.Fatalf("sts calls %d, keys %v", aws.stsCalls, aws.keys)
	}
}

func TestSnsStaticExpiredTokenIsPermanentWithItsReason(t *testing.T) {
	aws := &fakeAWS{statuses: []int{403}}
	api := httptest.NewServer(http.HandlerFunc(aws.sns))
	defer api.Close()
	c := NewSns(map[string]interface{}{"accessKeyId": "ASIAX",
		"secretAccessKey": "s", "sessionToken": "sess-tok-value",
		"topicArn": topic, "endpoint": api.URL}, "dev",
		transport.Dependencies{HTTPClient: api.Client(),
			Clock: clock.Func(func() time.Time { return credsNow })})
	err := c.SendMessage(context.Background(), "hi")
	if !transport.IsPermanent(err) ||
		!errors.Is(err, awscreds.ErrSessionTokenExpired) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "sess-tok-value") {
		t.Fatalf("error carries the token: %v", err)
	}
}

func TestSnsChinaRegionAndEndpointOverride(t *testing.T) {
	keys := func(extra map[string]interface{}) map[string]interface{} {
		m := map[string]interface{}{"accessKeyId": "a",
			"secretAccessKey": "s", "topicArn": "t"}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	deps := transport.Dependencies{HTTPClient: http.DefaultClient,
		Clock: clock.RealClock{}}
	c := NewSns(keys(map[string]interface{}{"region": "cn-north-1"}),
		"dev", deps)
	if c.url != "https://sns.cn-north-1.amazonaws.com.cn/" {
		t.Fatalf("url = %s", c.url)
	}
	c = NewSns(keys(map[string]interface{}{
		"endpoint": "https://sns.vpce.example.com/"}), "dev", deps)
	if c.url != "https://sns.vpce.example.com/" {
		t.Fatalf("url = %s", c.url)
	}
	if NewSns(keys(map[string]interface{}{"endpoint": "sns.local"}),
		"dev", deps) != nil {
		t.Fatal("an invalid endpoint was accepted")
	}
}

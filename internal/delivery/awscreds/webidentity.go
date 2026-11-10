package awscreds

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/signing"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// defaultSessionName names the role session when AWS_ROLE_SESSION_NAME
// is unset; it shows in CloudTrail.
const defaultSessionName = "kwatch"

// WebIdentityConfig is IAM roles for service accounts: the pod's
// projected token is exchanged at STS for the role's credentials.
type WebIdentityConfig struct {
	// Provider names the provider in errors, such as "SNS".
	Provider    string
	RoleARN     string
	TokenFile   string
	SessionName string
	// Endpoint is the STS URL; it must be https.
	Endpoint string
	Sender   transport.Sender
	Now      func() time.Time
}

// NewWebIdentity returns a Source that calls STS
// AssumeRoleWithWebIdentity. The request is not signed: the token is
// the proof.
func NewWebIdentity(c WebIdentityConfig) (Source, error) {
	if c.RoleARN == "" || c.TokenFile == "" {
		return nil, errors.New("web identity needs AWS_ROLE_ARN and " +
			"AWS_WEB_IDENTITY_TOKEN_FILE")
	}
	if !httpsEndpoint(c.Endpoint) {
		return nil, fmt.Errorf("STS endpoint %s must be an https URL",
			transport.LogURL(c.Endpoint))
	}
	if c.SessionName == "" {
		c.SessionName = defaultSessionName
	}
	klog.InfoS("using AWS web identity credentials",
		"provider", c.Provider, "roleArn", c.RoleARN,
		"sts", transport.LogURL(c.Endpoint))
	return newCached("web identity", c.Now, c.fetch), nil
}

func httpsEndpoint(raw string) bool {
	return transport.ValidEndpoint(raw) &&
		strings.HasPrefix(strings.TrimSpace(raw), "https://")
}

// stsAnswer is the part of the AssumeRoleWithWebIdentity answer kwatch
// reads.
type stsAnswer struct {
	Credentials struct {
		AccessKeyID     string    `xml:"AccessKeyId"`
		SecretAccessKey string    `xml:"SecretAccessKey"`
		SessionToken    string    `xml:"SessionToken"`
		Expiration      time.Time `xml:"Expiration"`
	} `xml:"AssumeRoleWithWebIdentityResult>Credentials"`
}

// stsError is the code of an STS error answer; the message is not read
// because it may quote the request.
type stsError struct {
	Code string `xml:"Error>Code"`
}

func (c WebIdentityConfig) fetch(
	ctx context.Context,
) (signing.Credentials, time.Time, error) {
	token, err := readToken(c.TokenFile)
	if err != nil {
		return signing.Credentials{}, time.Time{}, err
	}
	form := url.Values{}
	form.Set("Action", "AssumeRoleWithWebIdentity")
	form.Set("Version", "2011-06-15")
	form.Set("RoleArn", c.RoleARN)
	form.Set("RoleSessionName", c.SessionName)
	form.Set("WebIdentityToken", token)
	answer, err := c.Sender.Send(ctx, transport.Request{
		Provider: c.Provider + " STS", URL: c.Endpoint,
		Body:        []byte(form.Encode()),
		ContentType: "application/x-www-form-urlencoded",
	})
	if err != nil {
		var failed stsError
		if xml.Unmarshal(answer, &failed) == nil && failed.Code != "" {
			err = fmt.Errorf("%w (STS code %s)", err,
				safeCode(failed.Code))
		}
		return signing.Credentials{}, time.Time{}, fmt.Errorf(
			"assume role %s with web identity: %w", c.RoleARN, err)
	}
	var parsed stsAnswer
	if err := xml.Unmarshal(answer, &parsed); err != nil {
		return signing.Credentials{}, time.Time{}, errors.New(
			"STS answer is not valid AssumeRoleWithWebIdentity XML")
	}
	got := parsed.Credentials
	if got.AccessKeyID == "" || got.SecretAccessKey == "" {
		return signing.Credentials{}, time.Time{}, errors.New(
			"STS answer has no credentials")
	}
	return signing.Credentials{AccessKeyID: got.AccessKeyID,
		SecretAccessKey: got.SecretAccessKey,
		SessionToken:    got.SessionToken}, got.Expiration, nil
}

// safeCode keeps an error code to letters and digits, so an answer
// cannot smuggle text into the error.
func safeCode(code string) string {
	var b strings.Builder
	for _, r := range code {
		if r < 128 && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' ||
			r >= '0' && r <= '9' || r == '.') {
			b.WriteRune(r)
		}
		if b.Len() >= 64 {
			break
		}
	}
	return b.String()
}

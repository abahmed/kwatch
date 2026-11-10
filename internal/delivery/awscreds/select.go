package awscreds

import (
	"errors"
	"os"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/signing"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// The environment variables kwatch reads, as the AWS SDKs name them.
// EKS sets the first two for IAM roles for service accounts and the
// next two for EKS Pod Identity.
const (
	EnvRoleARN       = "AWS_ROLE_ARN"
	EnvTokenFile     = "AWS_WEB_IDENTITY_TOKEN_FILE"
	EnvSessionName   = "AWS_ROLE_SESSION_NAME"
	EnvAgentURL      = "AWS_CONTAINER_CREDENTIALS_FULL_URI"
	EnvAgentToken    = "AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE"
	EnvSTSEndpoint   = "AWS_ENDPOINT_URL_STS"
	sessionTokenHint = "it expires and kwatch cannot refresh it; use " +
		"IAM roles for service accounts or EKS Pod Identity"
)

// Options are what Select needs besides the provider's settings.
type Options struct {
	Provider string
	Region   string
	Sender   transport.Sender
	Now      func() time.Time
	// Getenv reads the environment; nil means os.Getenv.
	Getenv func(string) string
}

// Select picks the credential source, in the order the AWS SDKs use:
// accessKeyId and secretAccessKey from the settings, then web identity
// (AWS_ROLE_ARN with AWS_WEB_IDENTITY_TOKEN_FILE), then EKS Pod
// Identity (AWS_CONTAINER_CREDENTIALS_FULL_URI). It returns nil and no
// error when none is configured. The EC2 instance role is not used.
func Select(settings map[string]interface{}, o Options) (Source, error) {
	getenv := o.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	keyID := text(settings, "accessKeyId")
	secret := text(settings, "secretAccessKey")
	if keyID != "" || secret != "" {
		return static(o.Provider, keyID, secret,
			text(settings, "sessionToken"))
	}
	if arn := strings.TrimSpace(getenv(EnvRoleARN)); arn != "" {
		endpoint := strings.TrimSpace(getenv(EnvSTSEndpoint))
		if endpoint == "" {
			endpoint = ServiceURL("sts", o.Region)
		}
		return NewWebIdentity(WebIdentityConfig{
			Provider: o.Provider, RoleARN: arn,
			TokenFile:   strings.TrimSpace(getenv(EnvTokenFile)),
			SessionName: strings.TrimSpace(getenv(EnvSessionName)),
			Endpoint:    endpoint, Sender: o.Sender, Now: o.Now,
		})
	}
	if agent := strings.TrimSpace(getenv(EnvAgentURL)); agent != "" {
		return NewPodIdentity(PodIdentityConfig{
			Provider: o.Provider, URL: agent,
			TokenFile: strings.TrimSpace(getenv(EnvAgentToken)),
			Now:       o.Now,
		})
	}
	return nil, nil
}

func static(provider, keyID, secret, session string) (Source, error) {
	if keyID == "" || secret == "" {
		return nil, errors.New("set both accessKeyId and " +
			"secretAccessKey, or neither to use the pod's IAM role")
	}
	if session != "" {
		klog.InfoS("warning: sessionToken is set: "+sessionTokenHint,
			"provider", provider)
	}
	return Static{Keys: signing.Credentials{AccessKeyID: keyID,
		SecretAccessKey: secret, SessionToken: session}}, nil
}

func text(settings map[string]interface{}, key string) string {
	value, _ := settings[key].(string)
	return strings.TrimSpace(value)
}

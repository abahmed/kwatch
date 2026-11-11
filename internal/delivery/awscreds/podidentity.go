package awscreds

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/signing"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// maxAgentAnswerBytes caps the agent's JSON answer.
const maxAgentAnswerBytes = 64 << 10

// PodIdentityConfig is EKS Pod Identity: the node's agent hands out the
// role credentials for the pod's token.
type PodIdentityConfig struct {
	Provider  string
	URL       string
	TokenFile string
	Now       func() time.Time
}

// NewPodIdentity returns a Source that asks the Pod Identity agent at
// URL, which must name the agent or a loopback address.
func NewPodIdentity(c PodIdentityConfig) (Source, error) {
	if c.TokenFile == "" {
		return nil, errors.New("EKS Pod Identity needs " +
			"AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE")
	}
	if err := CheckAgentURL(c.URL); err != nil {
		return nil, err
	}
	klog.InfoS("using EKS Pod Identity credentials",
		"provider", c.Provider, "agent", transport.LogURL(c.URL))
	agent := podAgent{config: c, client: newAgentClient()}
	return newCached("pod identity", c.Now, agent.fetch), nil
}

type podAgent struct {
	config PodIdentityConfig
	client *http.Client
}

// agentAnswer is the agent's JSON credentials document.
type agentAnswer struct {
	AccessKeyID     string    `json:"AccessKeyId"`
	SecretAccessKey string    `json:"SecretAccessKey"`
	Token           string    `json:"Token"`
	Expiration      time.Time `json:"Expiration"`
}

func (a podAgent) fetch(
	ctx context.Context,
) (signing.Credentials, time.Time, error) {
	none := signing.Credentials{}
	token, err := readToken(a.config.TokenFile)
	if err != nil {
		return none, time.Time{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		a.config.URL, nil)
	if err != nil {
		return none, time.Time{}, errors.New(
			"build Pod Identity agent request")
	}
	req.Header.Set("Authorization", token)
	response, err := a.client.Do(req)
	if err != nil {
		return none, time.Time{}, fmt.Errorf(
			"call EKS Pod Identity agent: %w",
			transport.RedactURLError(err))
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return none, time.Time{}, fmt.Errorf(
			"EKS Pod Identity agent answered status %d",
			response.StatusCode)
	}
	var got agentAnswer
	body := io.LimitReader(response.Body, maxAgentAnswerBytes)
	if err := json.NewDecoder(body).Decode(&got); err != nil ||
		got.AccessKeyID == "" || got.SecretAccessKey == "" {
		return none, time.Time{}, errors.New(
			"EKS Pod Identity agent answer has no credentials")
	}
	return signing.Credentials{AccessKeyID: got.AccessKeyID,
		SecretAccessKey: got.SecretAccessKey,
		SessionToken:    got.Token}, got.Expiration, nil
}

//go:build e2e

package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

type ReceiverClient struct {
	environment *Environment
	client      *http.Client
}

type ReceiverRequest struct {
	ID       int             `json:"id"`
	Received time.Time       `json:"received"`
	Method   string          `json:"method"`
	Headers  http.Header     `json:"headers"`
	Status   int             `json:"status"`
	Body     []byte          `json:"body"`
	JSON     json.RawMessage `json:"json"`
}

func NewReceiverClient(environment *Environment) *ReceiverClient {
	return &ReceiverClient{
		environment: environment,
		client:      &http.Client{Timeout: 10 * time.Second},
	}
}

func (r *ReceiverClient) Requests(
	ctx context.Context,
) ([]ReceiverRequest, error) {
	var requests []ReceiverRequest
	if err := r.doJSON(
		ctx, http.MethodGet, "/requests", nil, &requests,
	); err != nil {
		return nil, err
	}
	return requests, nil
}

func (r *ReceiverClient) Clear(ctx context.Context) error {
	return r.doJSON(ctx, http.MethodDelete, "/requests", nil, nil)
}

func (r *ReceiverClient) SetPolicy(
	ctx context.Context,
	policy map[string]string,
) error {
	payload, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	return r.doJSON(ctx, http.MethodPost, "/control", payload, nil)
}

func (r *ReceiverClient) Policy(
	ctx context.Context,
) (map[string]any, error) {
	var policy map[string]any
	if err := r.doJSON(ctx, http.MethodGet, "/control", nil, &policy); err != nil {
		return nil, err
	}
	return policy, nil
}

func (r *ReceiverClient) WaitForCount(
	ctx context.Context,
	count int,
) ([]ReceiverRequest, error) {
	var requests []ReceiverRequest
	err := wait.PollUntilContextTimeout(ctx, 250*time.Millisecond,
		10*time.Minute, true, func(ctx context.Context) (bool, error) {
			current, err := r.Requests(ctx)
			if err != nil {
				return false, nil
			}
			requests = current
			return len(current) >= count, nil
		})
	return requests, err
}

func (r *ReceiverClient) doJSON(
	ctx context.Context,
	method, path string,
	payload []byte,
	result any,
) error {
	forward, err := StartPortForwardTarget(
		ctx,
		r.environment.Config.Kubeconfig,
		r.environment.Config.Context,
		r.environment.Config.ReceiverNamespace,
		"service/"+r.environment.Config.ReceiverService,
		8080,
	)
	if err != nil {
		return err
	}
	defer func() { _ = forward.Close() }()
	request, err := http.NewRequestWithContext(
		ctx, method, forward.URL(path), bytes.NewReader(payload),
	)
	if err != nil {
		return err
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := r.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK ||
		response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("receiver returned HTTP %d: %s",
			response.StatusCode, body)
	}
	if result == nil || response.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(response.Body).Decode(result)
}

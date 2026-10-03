//go:build e2e

package harness

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

type HealthClient struct {
	environment *Environment
	client      *http.Client
}

func NewHealthClient(environment *Environment) *HealthClient {
	return &HealthClient{
		environment: environment,
		client:      &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *HealthClient) Get(
	ctx context.Context,
	path string,
) ([]byte, int, error) {
	holder, err := h.environment.WaitForLeaseHolder(
		ctx, h.environment.Config.KwatchNamespace, "kwatch-leader",
	)
	if err != nil {
		return nil, 0, err
	}
	forward, err := StartPortForward(
		ctx,
		h.environment.Config.Kubeconfig,
		h.environment.Config.Context,
		h.environment.Config.KwatchNamespace,
		holder,
	)
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		_ = forward.Close()
	}()
	request, err := http.NewRequestWithContext(
		ctx, http.MethodGet, forward.URL(path), nil,
	)
	if err != nil {
		return nil, 0, err
	}
	response, err := h.client.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return nil, response.StatusCode, err
	}
	return body, response.StatusCode, nil
}

// AssertOK expects a 2xx answer. It retries while the Lease holder cannot be
// reached, because right after a handover the new Pod may not be running
// yet; an answer with a bad status is never retried.
func (h *HealthClient) AssertOK(ctx context.Context, path string) error {
	var status int
	var last error
	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, time.Minute, true,
		func(ctx context.Context) (bool, error) {
			_, code, err := h.Get(ctx, path)
			status, last = code, err
			return err == nil, nil
		})
	if err != nil {
		return fmt.Errorf("%s unreachable: %w", path, errors.Join(err, last))
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return fmt.Errorf("%s returned HTTP %d", path, status)
	}
	return nil
}

func (e *Environment) AssertHealthy(ctx context.Context) error {
	for _, path := range []string{"/healthz", "/readyz", "/availabilityz"} {
		if err := e.Health.AssertOK(ctx, path); err != nil {
			return err
		}
	}
	return nil
}

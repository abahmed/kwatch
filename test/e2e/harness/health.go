//go:build e2e

package harness

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

type HealthClient struct {
	environment *Environment
	client      *http.Client
}

type DiagnosticsClient struct {
	Health *HealthClient
}

func NewHealthClient(environment *Environment) *HealthClient {
	return &HealthClient{
		environment: environment,
		client:      &http.Client{Timeout: 10 * time.Second},
	}
}

func NewDiagnosticsClient(environment *Environment) *DiagnosticsClient {
	return &DiagnosticsClient{Health: environment.Health}
}

func (h *HealthClient) Get(
	ctx context.Context,
	path string,
	diagnostics bool,
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
	if diagnostics {
		request.Header.Set("Authorization", "Bearer "+
			h.environment.Config.DiagnosticsToken)
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

func (h *HealthClient) AssertOK(ctx context.Context, path string) error {
	_, status, err := h.Get(ctx, path, false)
	if err != nil {
		return err
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

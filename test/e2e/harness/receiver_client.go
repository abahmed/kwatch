//go:build e2e

package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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

type DeliveryMatch struct {
	Name   string
	Reason string
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

func (r *ReceiverClient) Matching(
	ctx context.Context,
	match DeliveryMatch,
) ([]ReceiverRequest, error) {
	requests, err := r.Requests(ctx)
	if err != nil {
		return nil, err
	}
	matched := make([]ReceiverRequest, 0, len(requests))
	for _, request := range requests {
		var payload map[string]any
		if json.Unmarshal(request.JSON, &payload) != nil {
			continue
		}
		if match.Name != "" && !titleNames(payload, match.Name) {
			continue
		}
		if match.Reason != "" && !routeHasReason(payload, match.Reason) {
			continue
		}
		matched = append(matched, request)
	}
	return matched, nil
}

// titleNames reports whether the notification title mentions name as a
// whole word, such as "persistent is failing in <namespace>". A roll-up
// title only counts the problems, so its listed lines and note, which
// carry each problem's own title, are searched too.
func titleNames(payload map[string]any, name string) bool {
	title, _ := payload["title"].(string)
	if hasWord(title, name) {
		return true
	}
	key, _ := payload["key"].(string)
	if !strings.HasPrefix(key, "rollup/") {
		return false
	}
	note, _ := payload["note"].(string)
	if hasWord(note, name) {
		return true
	}
	lines, _ := payload["lines"].([]any)
	for _, line := range lines {
		text, _ := line.(string)
		if hasWord(text, name) {
			return true
		}
	}
	return false
}

// hasWord reports whether text contains name as a whole word.
func hasWord(text, name string) bool {
	for _, word := range strings.Fields(text) {
		if strings.Trim(word, ".,:;()") == name {
			return true
		}
	}
	return false
}

// routeHasReason reports whether the notification route lists reason.
func routeHasReason(payload map[string]any, reason string) bool {
	route, _ := payload["route"].(map[string]any)
	reasons, _ := route["Reasons"].([]any)
	for _, item := range reasons {
		if item == reason {
			return true
		}
	}
	return false
}

func (r *ReceiverClient) WaitForMatchCount(
	ctx context.Context,
	match DeliveryMatch,
	count int,
) ([]ReceiverRequest, error) {
	var requests []ReceiverRequest
	err := wait.PollUntilContextTimeout(ctx, 250*time.Millisecond,
		10*time.Minute, true, func(ctx context.Context) (bool, error) {
			current, err := r.Matching(ctx, match)
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

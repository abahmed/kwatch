package license

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DefaultEndpoint is the license server.
const DefaultEndpoint = "https://api.kwatch.dev"

const (
	defaultUserAgent = "kwatch-license/1"
	// maxResponseBytes caps how much of a response body is read.
	maxResponseBytes = 64 << 10
	maxRetryAfter    = 24 * time.Hour
)

// Server verdicts and request problems returned by Client methods.
var (
	ErrRevoked       = errors.New("license: revoked by server")
	ErrSuspended     = errors.New("license: suspended by server")
	ErrServerExpired = errors.New("license: expired according to server")
	ErrTrialUsed     = errors.New("license: license already used " +
		"on this cluster")
	ErrKeyReplaced        = errors.New("license: key replaced by a newer key")
	ErrInstanceSuperseded = errors.New("license: another instance " +
		"holds this license")
	ErrProofMismatch = errors.New("license: cluster proof mismatch")
	ErrUnauthorized  = errors.New("license: server does not know " +
		"this binding")
	ErrStaleTimestamp = errors.New("license: request timestamp " +
		"rejected by server")
)

// ErrConflict means the license is bound to another cluster.
type ErrConflict struct {
	BoundSince time.Time // zero when unknown
}

func (e ErrConflict) Error() string {
	if e.BoundSince.IsZero() {
		return "license: bound to another cluster"
	}
	return "license: bound to another cluster " +
		"since " + e.BoundSince.UTC().Format(time.RFC3339)
}

// ErrTransferLimit means the license used up its moves between clusters
// (the web transfer, deactivate and move share one limit).
type ErrTransferLimit struct {
	RetryAfter time.Duration // until a slot frees; zero when unknown
}

func (e ErrTransferLimit) Error() string {
	if e.RetryAfter <= 0 {
		return "license: transfer limit reached"
	}
	return "license: transfer limit reached, a slot frees in " +
		e.RetryAfter.Round(time.Hour).String()
}

// ErrRateLimited means the server asked the client to slow down.
type ErrRateLimited struct {
	RetryAfter time.Duration
}

func (e ErrRateLimited) Error() string {
	return "license: rate limited, retry after " + e.RetryAfter.String()
}

// TransientError wraps network failures, 5xx responses and malformed 2xx
// bodies: conditions where retrying later is the right response.
type TransientError struct {
	Status int // 0 for network errors
	Err    error
}

func (e *TransientError) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("license: server error (status %d): %v", e.Status, e.Err)
	}
	return "license: request failed: " + e.Err.Error()
}

func (e *TransientError) Unwrap() error { return e.Err }

// StatusError is a response the client does not otherwise classify (400, 404,
// redirects and so on). It is not transient.
type StatusError struct {
	Status int
	Code   string
}

func (e *StatusError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("license: unexpected response %d %s", e.Status, e.Code)
	}
	return fmt.Sprintf("license: unexpected response %d", e.Status)
}

// IsTransient reports whether err is worth retrying with backoff: network
// errors, 5xx, rate limiting and timestamp rejection.
func IsTransient(err error) bool {
	var t *TransientError
	var r ErrRateLimited
	return errors.As(err, &t) || errors.As(err, &r) ||
		errors.Is(err, ErrStaleTimestamp)
}

// Client talks to the license server. The caller supplies HTTP (proxy and CA
// configuration); redirects are never followed.
type Client struct {
	Endpoint  string // default DefaultEndpoint
	HTTP      *http.Client
	UserAgent string // default "kwatch-license/1"
}

// Result is a successful (or, with an error, a failed) server response.
// The response body's srv_time is unsigned and deliberately not surfaced:
// only the srv_time inside a verified lease may move the clock guard.
type Result struct {
	Lease string
}

// ActivateRequest is the body of POST /license/activate.
type ActivateRequest struct {
	Key          string
	ClusterUUID  string
	ClusterProof string
	InstancePub  string
	Version      string
	NodeCount    int
}

// SignedRequest is the body of the instance-signed renew and deactivate calls.
type SignedRequest struct {
	LID          string
	ClusterUUID  string
	ClusterProof string
	LeaseIat     int64
	TS           time.Time
	Version      string
	NodeCount    int
}

type activateBody struct {
	Key          string `json:"key"`
	ClusterUUID  string `json:"cluster_uuid"`
	ClusterProof string `json:"cluster_proof"`
	InstancePub  string `json:"instance_pub"`
	Nonce        string `json:"nonce"`
	Version      string `json:"kwatch_version,omitempty"`
	NodeCount    int    `json:"node_count"`
}

type signedBody struct {
	LID          string `json:"lid"`
	ClusterUUID  string `json:"cluster_uuid"`
	ClusterProof string `json:"cluster_proof"`
	LeaseIat     int64  `json:"lease_iat"`
	TS           int64  `json:"ts"`
	Version      string `json:"kwatch_version,omitempty"`
	NodeCount    int    `json:"node_count,omitempty"`
}

type wireResponse struct {
	Lease      string `json:"lease"`
	Error      string `json:"error"`
	BoundSince int64  `json:"bound_since"`
	// RetryAfter is seconds until a transfer slot frees (transfer_limit).
	RetryAfter int64 `json:"retry_after"`
}

// Activate binds the license to this cluster and instance.
func (c *Client) Activate(
	ctx context.Context, req ActivateRequest,
) (Result, error) {
	return c.activateCall(ctx, "/license/activate", req)
}

// Move takes the license over from the cluster it is bound to. It has the
// body and the responses of Activate, and adds ErrTransferLimit.
func (c *Client) Move(
	ctx context.Context, req ActivateRequest,
) (Result, error) {
	return c.activateCall(ctx, "/license/move", req)
}

func (c *Client) activateCall(
	ctx context.Context, path string, req ActivateRequest,
) (Result, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return Result{}, err
	}
	body, err := json.Marshal(activateBody{
		Key: req.Key, ClusterUUID: req.ClusterUUID, ClusterProof: req.ClusterProof,
		InstancePub: req.InstancePub,
		Nonce:       base64.RawURLEncoding.EncodeToString(nonce),
		Version:     req.Version, NodeCount: req.NodeCount,
	})
	if err != nil {
		return Result{}, err
	}
	return c.call(ctx, path, body, "", true)
}

// Renew fetches a fresh lease, authenticated by priv (the instance key).
func (c *Client) Renew(
	ctx context.Context, priv ed25519.PrivateKey, req SignedRequest,
) (Result, error) {
	return c.signed(ctx, "/license/renew", priv, req, true)
}

// Deactivate releases the cluster binding, authenticated by priv.
func (c *Client) Deactivate(
	ctx context.Context, priv ed25519.PrivateKey, req SignedRequest,
) (Result, error) {
	return c.signed(ctx, "/license/deactivate", priv, req, false)
}

func (c *Client) signed(
	ctx context.Context, path string, priv ed25519.PrivateKey,
	req SignedRequest, wantLease bool,
) (Result, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return Result{}, errors.New("license: invalid instance key")
	}
	body, err := json.Marshal(signedBody{
		LID: req.LID, ClusterUUID: req.ClusterUUID, ClusterProof: req.ClusterProof,
		LeaseIat: req.LeaseIat, TS: req.TS.Unix(),
		Version: req.Version, NodeCount: req.NodeCount,
	})
	if err != nil {
		return Result{}, err
	}
	sig := base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv, body))
	return c.call(ctx, path, body, sig, wantLease)
}

func (c *Client) httpClient() *http.Client {
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	cp := *hc
	cp.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &cp
}

func (c *Client) call(
	ctx context.Context, path string, body []byte,
	sig string, wantLease bool,
) (Result, error) {
	endpoint := strings.TrimRight(c.Endpoint, "/")
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, endpoint+path, bytes.NewReader(body),
	)
	if err != nil {
		return Result{}, err
	}
	ua := c.UserAgent
	if ua == "" {
		ua = defaultUserAgent
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", ua)
	if sig != "" {
		req.Header.Set("X-Kwatch-Sig", sig)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return Result{}, &TransientError{Err: err}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return Result{}, &TransientError{Status: resp.StatusCode, Err: err}
	}
	res := Result{}
	if resp.StatusCode == http.StatusNoContent && !wantLease {
		return res, nil
	}
	var w wireResponse
	parseErr := json.Unmarshal(raw, &w)
	if resp.StatusCode == http.StatusOK {
		if parseErr != nil || (wantLease && w.Lease == "") {
			return res, &TransientError{
				Status: resp.StatusCode,
				Err:    errors.New("malformed response body"),
			}
		}
		res.Lease = w.Lease
		return res, nil
	}
	return res, classify(resp.StatusCode, w, resp.Header.Get("Retry-After"))
}

func classify(status int, w wireResponse, retryAfter string) error {
	switch {
	case status >= 500:
		return &TransientError{Status: status, Err: fmt.Errorf("status %d", status)}
	case status == http.StatusTooManyRequests && w.Error == "transfer_limit":
		return ErrTransferLimit{
			RetryAfter: time.Duration(max(w.RetryAfter, 0)) * time.Second,
		}
	case status == http.StatusTooManyRequests:
		return ErrRateLimited{RetryAfter: parseRetryAfter(retryAfter)}
	case status == http.StatusUnauthorized:
		return ErrUnauthorized
	}
	switch w.Error {
	case "bound_to_other_cluster":
		var since time.Time
		if w.BoundSince > 0 {
			since = time.Unix(w.BoundSince, 0).UTC()
		}
		return ErrConflict{BoundSince: since}
	case "instance_superseded":
		return ErrInstanceSuperseded
	case "revoked":
		return ErrRevoked
	case "suspended":
		return ErrSuspended
	case "expired":
		return ErrServerExpired
	case "trial_already_used":
		return ErrTrialUsed
	case "key_replaced":
		return ErrKeyReplaced
	case "cluster_proof_mismatch":
		return ErrProofMismatch
	case "stale_timestamp":
		return ErrStaleTimestamp
	}
	return &StatusError{Status: status, Code: w.Error}
}

// parseRetryAfter accepts delta-seconds or an HTTP date; unparseable values
// yield a minute.
func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	d := time.Minute
	if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
		d = time.Duration(secs) * time.Second
	} else if t, err := http.ParseTime(v); err == nil {
		d = time.Until(t)
	}
	if d < 0 {
		d = 0
	}
	return min(d, maxRetryAfter)
}

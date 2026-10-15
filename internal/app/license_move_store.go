package app

import (
	"context"
	"encoding/json"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/license"
)

const (
	// licenseMoveRequestField holds the RFC 3339 time `kwatch license
	// move` asked this cluster to take the license over. The running
	// leader, a separate process, performs the move and removes it.
	licenseMoveRequestField = "move-requested-at"
	// licenseMoveResultField holds the leader's answer as JSON, so the
	// command can print how its request ended.
	licenseMoveResultField = "move-result"
)

// moveResult is the content of licenseMoveResultField.
type moveResult struct {
	// RequestedAt echoes the request this answers.
	RequestedAt string `json:"requested_at"`
	Result      string `json:"result"`
	RetryAfterS int64  `json:"retry_after_s,omitempty"`
	Detail      string `json:"detail,omitempty"`
}

// MarkMoveRequested records a move request for the leader and returns the
// stored marker, which identifies the request in its answer.
func (s kubeInstanceStore) MarkMoveRequested(
	ctx context.Context, at time.Time,
) (string, error) {
	existing, err := s.secrets().Get(
		ctx, licenseInstanceSecret, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	if existing.Data == nil {
		existing.Data = map[string][]byte{}
	}
	marker := at.UTC().Format(time.RFC3339Nano)
	existing.Data[licenseMoveRequestField] = []byte(marker)
	delete(existing.Data, licenseMoveResultField)
	_, err = s.secrets().Update(ctx, existing, metav1.UpdateOptions{})
	return marker, err
}

// MoveRequestedAt returns the time of the pending move request, zero when
// there is none or the Secret does not exist.
func (s kubeInstanceStore) MoveRequestedAt(
	ctx context.Context,
) (time.Time, error) {
	secret, err := s.secrets().Get(
		ctx, licenseInstanceSecret, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	raw := secret.Data[licenseMoveRequestField]
	if len(raw) == 0 {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, string(raw))
}

// FinishMove answers the pending request and removes it. A move that left
// this cluster holding the license also removes the release marker, so a
// later session does not read the old release as current.
func (s kubeInstanceStore) FinishMove(
	ctx context.Context, out license.MoveOutcome,
) error {
	existing, err := s.secrets().Get(
		ctx, licenseInstanceSecret, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if existing.Data == nil {
		existing.Data = map[string][]byte{}
	}
	raw, err := json.Marshal(moveResult{
		RequestedAt: string(existing.Data[licenseMoveRequestField]),
		Result:      out.Result,
		RetryAfterS: int64(out.RetryAfter / time.Second),
		Detail:      out.Detail,
	})
	if err != nil {
		return err
	}
	existing.Data[licenseMoveResultField] = raw
	delete(existing.Data, licenseMoveRequestField)
	if out.Result == license.MoveMoved ||
		out.Result == license.MoveAlreadyHere {
		delete(existing.Data, licenseDeactivatedField)
	}
	_, err = s.secrets().Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

// MoveResult returns the answer to the request identified by marker, and
// false while there is none yet.
func (s kubeInstanceStore) MoveResult(
	ctx context.Context, marker string,
) (moveResult, bool, error) {
	secret, err := s.secrets().Get(
		ctx, licenseInstanceSecret, metav1.GetOptions{})
	if err != nil {
		return moveResult{}, false, err
	}
	var res moveResult
	raw := secret.Data[licenseMoveResultField]
	if len(raw) == 0 || json.Unmarshal(raw, &res) != nil ||
		res.RequestedAt != marker {
		return moveResult{}, false, nil
	}
	return res, true, nil
}

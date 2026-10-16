package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/abahmed/kwatch/internal/license"
)

const (
	// licenseMoveWait bounds how long `kwatch license move` waits for the
	// running leader, which looks for a request once a minute.
	licenseMoveWait = 150 * time.Second
	// licenseMovePoll is how often the command looks for the answer.
	licenseMovePoll = 2 * time.Second
)

// LicenseMove implements `kwatch license move`: it asks the running kwatch
// leader to take the license over from the cluster it is bound to. It runs
// inside the kwatch pod, like `license deactivate`.
func LicenseMove(out, errOut io.Writer) int {
	ctx, cancel := context.WithTimeout(
		context.Background(), licenseMoveWait)
	defer cancel()
	return defaultLicenseCLI().move(ctx, out, errOut)
}

// move writes a request to the instance Secret. Only the leader holds the
// lease, so it performs the move and answers in the same Secret.
func (c licenseCLI) move(ctx context.Context, out, errOut io.Writer) int {
	cfg, err := c.loadConfig()
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "ERROR: load configuration: %v\n", err)
		return 1
	}
	if cfg.License.Key == "" {
		_, _ = fmt.Fprintln(errOut,
			"ERROR: no license key is configured; nothing to move.")
		return 1
	}
	key, err := license.ParseKey(cfg.License.Key)
	if err != nil {
		_, _ = fmt.Fprintf(errOut,
			"ERROR: the configured license key is not valid: %v\n", err)
		return 1
	}
	kube, err := c.kube()
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "ERROR: connect to the Kubernetes API "+
			"(run this inside the kwatch pod): %v\n", err)
		return 1
	}
	store := kubeInstanceStore{client: kube, namespace: c.namespace}
	if _, err := license.EnsureInstanceKey(ctx, store); err != nil {
		_, _ = fmt.Fprintf(errOut, "ERROR: instance key: %v\n", err)
		return 1
	}
	marker, err := store.MarkMoveRequested(ctx, c.now())
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "ERROR: ask kwatch to move the "+
			"license: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "Moving license %s to this cluster...\n",
		license.MaskLID(key.LID))
	res, err := c.awaitMove(ctx, store, marker)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "ERROR: %v\n", err)
		return 1
	}
	return printMoveResult(out, errOut, key, res)
}

// awaitMove polls for the answer to the request marked by marker.
func (c licenseCLI) awaitMove(
	ctx context.Context, store kubeInstanceStore, marker string,
) (moveResult, error) {
	sleep := c.sleep
	if sleep == nil {
		sleep = sleepLicense
	}
	for {
		res, ok, err := store.MoveResult(ctx, marker)
		if err != nil {
			return moveResult{}, fmt.Errorf("read the answer: %v", err)
		}
		if ok {
			return res, nil
		}
		if err := sleep(ctx, licenseMovePoll); err != nil {
			return moveResult{}, errors.New("no answer from the running " +
				"kwatch yet. Is kwatch running with this license key? " +
				"Only its leader answers; the request stays queued for " +
				"10 minutes. Check `kwatch license status` or retry")
		}
	}
}

func printMoveResult(
	out, errOut io.Writer, key license.Key, res moveResult,
) int {
	lid := license.MaskLID(key.LID)
	switch res.Result {
	case license.MoveMoved:
		_, _ = fmt.Fprintf(out, "License %s is now active on this "+
			"cluster. The previous cluster drops to Community at its next "+
			"daily check.\n", lid)
		return 0
	case license.MoveAlreadyHere:
		_, _ = fmt.Fprintf(out, "License %s is already active on this "+
			"cluster; nothing to move.\n", lid)
		return 0
	case license.MoveTransferLimit:
		_, _ = fmt.Fprintf(errOut, "ERROR: transfer limit reached: "+
			"a license can move 3 times in 30 days. %s\n",
			slotText(time.Duration(res.RetryAfterS)*time.Second))
	case license.MoveInvalidKey:
		_, _ = fmt.Fprintln(errOut, "ERROR: the license server does not "+
			"accept this key. Check it, or regenerate it at "+
			"https://kwatch.dev/account")
	case license.MoveStale:
		_, _ = fmt.Fprintln(errOut, "ERROR: the request waited too long "+
			"for the running kwatch and was dropped; run it again.")
	default:
		_, _ = fmt.Fprintf(errOut, "ERROR: the license could not be "+
			"moved: %s\n", res.Detail)
	}
	return 1
}

// slotText says when the next transfer slot frees up.
func slotText(d time.Duration) string {
	if d <= 0 {
		return "A slot frees up as the oldest move ages past 30 days."
	}
	days := int(math.Ceil(d.Hours() / 24))
	if days == 1 {
		return "A slot frees up in 1 day."
	}
	return fmt.Sprintf("A slot frees up in %d days.", days)
}

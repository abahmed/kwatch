package app

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/license"
	"github.com/abahmed/kwatch/internal/storage"
)

// EffectiveEntitlements is the edition the running installation uses, for
// commands that run apart from it (`kwatch version --json`,
// `kwatch replay-export`). Such a command is a second process with no
// license manager of its own, so it reads what the leader left in the
// state file: the edition the last session ran and the cached lease. The
// edition counts as Pro only while that lease is still inside its grace;
// a missing or unreadable state file reads as Community, never as Pro.
func EffectiveEntitlements(
	statePath string, now time.Time,
) feature.Entitlements {
	base := ResolveEntitlements()
	if base.Edition() == feature.ProEdition {
		return base
	}
	if stateRunsPro(statePath, now) {
		return feature.ProEntitlements()
	}
	return base
}

// StartEntitlements is the edition the process starts its first
// generation with: the build's own, else Pro when the state file says the
// last session ran Pro and its cached lease is still inside its grace.
// The leader's license session then settles the edition as before; a
// start that guessed wrong rebuilds once, as every start did when it
// always began as Community. A Pro installation thus builds once, not
// twice. The file is opened in place, read-only: before the first
// generation nothing in this process holds it, and a holder elsewhere
// makes the open time out, which reads as the build's edition.
func StartEntitlements(
	statePath string, now time.Time,
) feature.Entitlements {
	base := ResolveEntitlements()
	if base.Edition() == feature.ProEdition {
		return base
	}
	if _, err := os.Stat(statePath); err != nil {
		return base
	}
	if fileRunsPro(statePath, now) {
		return feature.ProEntitlements()
	}
	return base
}

// cachedLeaseVerifier checks the signature of the lease a state file
// holds before its edition counts: a file edited by hand never starts or
// unlocks Pro. Tests replace it with their own key.
var cachedLeaseVerifier = license.DefaultVerifier

// stateRunsPro reads a copy of the state file. The running leader holds
// the file locked, and a read-only open waits for that same lock, so the
// file cannot be read in place. Copies an earlier command left behind are
// removed first.
func stateRunsPro(statePath string, now time.Time) bool {
	sweepEditionCopies(statePath, now)
	copyPath, err := copyStateFile(statePath)
	if err != nil {
		return false
	}
	defer func() { _ = os.Remove(copyPath) }()
	return fileRunsPro(copyPath, now)
}

// fileRunsPro opens path read-only and reports whether it records a Pro
// session whose cached lease is still inside its grace.
func fileRunsPro(path string, now time.Time) bool {
	store, err := storage.Open(path, storage.Options{
		Now: func() time.Time { return now }, DeferRepair: true,
	})
	if err != nil {
		return false
	}
	defer func() { _ = store.Close() }()
	disk := diskState{store: store}
	var last editionRecord
	found, err := disk.get(stateEditionLast, &last)
	if err != nil || !found || last.Edition != string(feature.ProEdition) {
		return false
	}
	lease, _, err := boltLeaseStore{disk: disk}.LoadLease()
	if err != nil {
		return false
	}
	grace, ok := license.CachedLeaseGraceUntil(cachedLeaseVerifier(), lease)
	return ok && now.Before(grace)
}

// copyStateFile copies the state file next to itself, on the data volume,
// which is writable when the root file system is not.
func copyStateFile(statePath string) (string, error) {
	in, err := os.Open(statePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = in.Close() }()
	out, err := os.CreateTemp(
		filepath.Dir(statePath), editionCopyPrefix+"*")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(out.Name())
		return "", err
	}
	return out.Name(), out.Close()
}

const (
	// editionCopyPrefix starts the name of every copy copyStateFile makes.
	editionCopyPrefix = "state.edition"
	// editionCopyMaxAge is how old a copy must be to count as left
	// behind. A command uses its copy for seconds; the margin keeps a
	// sweep from removing the copy of a command that runs right now.
	editionCopyMaxAge = 10 * time.Minute
)

// sweepEditionCopies removes the copies that interrupted commands left
// next to the state file (a `kubectl exec` that was cut, a killed
// process). Each is as large as the state file and nothing else deletes
// it, so on a small volume a few of them leave no room for the state
// itself. It runs before every copy and when kwatch starts.
func sweepEditionCopies(statePath string, now time.Time) {
	dir := filepath.Dir(statePath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() ||
			!strings.HasPrefix(entry.Name(), editionCopyPrefix) {
			continue
		}
		info, err := entry.Info()
		if err != nil || now.Sub(info.ModTime()) < editionCopyMaxAge {
			continue
		}
		_ = os.Remove(filepath.Join(dir, entry.Name()))
	}
}

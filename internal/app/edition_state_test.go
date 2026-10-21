package app

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/license"
	"github.com/abahmed/kwatch/internal/storage"
)

var stateNow = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// stateFileWith writes a state file as the leader leaves it: the edition
// the last session ran and the cached lease (empty for none).
func stateFileWith(
	t *testing.T, edition feature.Edition, exp time.Time,
) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := storage.Open(path, storage.Options{
		Now: func() time.Time { return stateNow }})
	require.NoError(t, err)
	_, err = store.Claim()
	require.NoError(t, err)
	disk := diskState{store: store}
	require.NoError(t, disk.put(stateEditionLast,
		editionRecord{Edition: string(edition)}))
	if !exp.IsZero() {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		lease := license.SignLease(license.Lease{
			KID: "v1", LID: "lic_x", Plan: license.PlanPro,
			Nbf: stateNow.Add(-time.Hour).Unix(), Exp: exp.Unix(),
		}, priv)
		require.NoError(t, boltLeaseStore{disk: disk}.SaveLease(
			lease, license.ClockGuard{}))
	}
	require.NoError(t, store.Close())
	return path
}

// replay-export and version --json run apart from the leader, so they
// read its edition from the state file.
func TestEffectiveEntitlementsReadTheLeadersProEdition(t *testing.T) {
	path := stateFileWith(t, feature.ProEdition, stateNow.Add(48*time.Hour))

	got := EffectiveEntitlements(path, stateNow)

	require.Equal(t, feature.ProEdition, got.Edition())
	require.True(t, got.Has(FlightRecorder))
}

func TestEffectiveEntitlementsAreCommunityPastTheLeaseGrace(t *testing.T) {
	past := stateNow.Add(-license.Grace - time.Hour)
	path := stateFileWith(t, feature.ProEdition, past)

	require.Equal(t, feature.CommunityEdition,
		EffectiveEntitlements(path, stateNow).Edition())
}

func TestEffectiveEntitlementsKeepGraceAfterTheLeaseExpires(t *testing.T) {
	path := stateFileWith(t, feature.ProEdition, stateNow.Add(-time.Hour))

	require.Equal(t, feature.ProEdition,
		EffectiveEntitlements(path, stateNow).Edition())
}

func TestEffectiveEntitlementsAreCommunityForACommunityLeader(t *testing.T) {
	path := stateFileWith(t, feature.CommunityEdition, stateNow.Add(48*time.Hour))

	require.Equal(t, feature.CommunityEdition,
		EffectiveEntitlements(path, stateNow).Edition())
}

func TestEffectiveEntitlementsAreCommunityWithoutALease(t *testing.T) {
	path := stateFileWith(t, feature.ProEdition, time.Time{})

	require.Equal(t, feature.CommunityEdition,
		EffectiveEntitlements(path, stateNow).Edition())
}

func TestEffectiveEntitlementsFailClosed(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "none.db")
	junk := filepath.Join(dir, "junk.db")
	require.NoError(t, os.WriteFile(junk, []byte("not a state file"), 0o600))

	for name, path := range map[string]string{
		"missing": missing, "unreadable": junk,
	} {
		require.Equal(t, feature.CommunityEdition,
			EffectiveEntitlements(path, stateNow).Edition(), name)
	}
}

func TestEffectiveEntitlementsLeaveTheStateFileAndDirectoryAlone(t *testing.T) {
	path := stateFileWith(t, feature.ProEdition, stateNow.Add(48*time.Hour))
	before, err := os.ReadFile(path)
	require.NoError(t, err)

	EffectiveEntitlements(path, stateNow)

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.True(t, string(before) == string(after),
		"the file is read, not written")
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, entries, 1, "the temporary copy is removed")
}

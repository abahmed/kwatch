package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	bolt "go.etcd.io/bbolt"
	"k8s.io/klog/v2"
)

// ResetMigrationFailed means a migration step of an older file failed:
// its transaction rolled back and the file was reset as before.
const ResetMigrationFailed = "migration_failed"

// migration upgrades a state file by one schema version, from From to
// From+1. A step runs inside the write transaction that opens the file
// for writing, with every other step of the chain and the version bump,
// so a file is either fully migrated or left as it was. A step must be
// idempotent: it may see data it already converted.
type migration struct {
	From uint64
	Step func(tx *bolt.Tx) error
}

// migrations are the steps from each older schema version, one per
// version. An older file with a complete chain to SchemaVersion is
// migrated in place at Open; a file older than the first step, newer
// than SchemaVersion, or whose chain fails is reset (reset.go). There is
// no step yet: version 2 is the first that keeps its files across a
// layout change. When a change bumps SchemaVersion, add the step from
// the previous version here instead of resetting every installation.
var migrations []migration

// migrationChain returns the steps that take version to SchemaVersion,
// in order, and whether the chain is complete.
func migrationChain(version uint64) ([]migration, bool) {
	if version >= SchemaVersion {
		return nil, version == SchemaVersion
	}
	byFrom := make(map[uint64]migration, len(migrations))
	for _, m := range migrations {
		byFrom[m.From] = m
	}
	var chain []migration
	for v := version; v < SchemaVersion; v++ {
		m, ok := byFrom[v]
		if !ok || m.Step == nil {
			return nil, false
		}
		chain = append(chain, m)
	}
	return chain, true
}

// migrateOlder migrates tx's file when it is older than SchemaVersion.
func migrateOlder(tx *bolt.Tx) error {
	version, known := storedVersion(tx)
	if !known || version >= SchemaVersion {
		return nil
	}
	return migrate(tx, version)
}

// migrate runs the chain from version inside tx. A failed step returns
// an *unusableError, which resets the file like any unusable one; the
// transaction rolls back first, so the reset sets aside the file as it
// was, not a half-converted one.
func migrate(tx *bolt.Tx, version uint64) error {
	chain, ok := migrationChain(version)
	if !ok {
		return &unusableError{
			reason: ResetSchemaMismatch, version: version,
			cause: fmt.Errorf("supported version is %d", SchemaVersion),
		}
	}
	for _, m := range chain {
		if err := m.Step(tx); err != nil {
			return &unusableError{
				reason: ResetMigrationFailed, version: version,
				cause: fmt.Errorf("migrate %d to %d: %w",
					m.From, m.From+1, err),
			}
		}
	}
	if len(chain) > 0 {
		klog.InfoS("state store migrated", "component", "state",
			"operation", "migrate", "fromVersion", version,
			"toVersion", SchemaVersion, "steps", len(chain))
	}
	return nil
}

// backupSuffix starts the name of the copy of a state file taken before
// it is migrated: path + backupSuffix + "<version>".
const backupSuffix = ".pre-migration-v"

// backupBeforeMigration copies db's file next to it before a migration
// changes it, when the volume has room for the copy and its growth (free
// is the store's free space, which honours a volume limit; nil asks the
// operating system): the migration is one transaction, so the copy is
// for inspecting a migration that succeeded with a bug, not for a
// failure. Only the latest such copy is kept. No room skips the copy
// with a log line.
func backupBeforeMigration(db *bolt.DB, path string, free freeSpaceFunc) {
	var version uint64
	var known bool
	_ = db.View(func(tx *bolt.Tx) error {
		version, known = storedVersion(tx)
		return nil
	})
	if chain, ok := migrationChain(version); !known || !ok ||
		len(chain) == 0 {
		return
	}
	if free == nil {
		free = osFreeSpace
	}
	info, err := os.Stat(path)
	room, ferr := free(path)
	if err != nil || ferr != nil || uint64(info.Size())*2 > room {
		klog.InfoS("state store migrating without a backup copy: not "+
			"enough free space", "component", "state",
			"operation", "migrate", "fromVersion", version)
		return
	}
	dst := fmt.Sprintf("%s%s%d", path, backupSuffix, version)
	// Copied under a temporary name first: an earlier copy is removed
	// only once the new one is complete, so a failed copy (disk full)
	// never leaves no copy at all.
	tmp := dst + backupTemp
	// A copy a crash left half written is of no use: it only takes room.
	pruneBackups(path, "", true)
	err = db.View(func(tx *bolt.Tx) error {
		return tx.CopyFile(tmp, 0o600)
	})
	if err != nil {
		_ = os.Remove(tmp)
		klog.ErrorS(err, "state store backup before migration failed",
			"component", "state", "operation", "migrate")
		return
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		klog.ErrorS(err, "state store backup before migration failed",
			"component", "state", "operation", "migrate")
		return
	}
	// Earlier copies go only once the new one is in place.
	pruneBackups(path, dst, false)
	klog.InfoS("state store backed up before migration",
		"component", "state", "operation", "migrate", "keptAs", dst)
}

// backupTemp names a pre-migration copy still being written.
const backupTemp = ".tmp"

// pruneBackups removes pre-migration copies of path other than keep:
// with partial, only copies a crash left half written (.tmp); without,
// only complete ones, so at most one stays on the volume.
func pruneBackups(path, keep string, partial bool) {
	names, err := filepath.Glob(path + backupSuffix + "*")
	if err != nil {
		return
	}
	sort.Strings(names)
	for _, name := range names {
		if name != keep && strings.HasSuffix(name, backupTemp) == partial {
			_ = os.Remove(name)
		}
	}
}

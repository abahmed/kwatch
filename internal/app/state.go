package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/storage"
)

// State keys in the store's state bucket.
const (
	stateClusterID      = "cluster.id"
	stateInitialized    = "initialized"
	stateVersion        = "version"
	stateLastSeen       = "last.seen"
	stateSession        = "runtime.session"
	stateAnnouncement   = "startup.announcement"
	stateTelemetrySent  = "telemetry.last.sent"
	stateNotifiedUpdate = "upgrade.notified.version"
)

// diskState keeps kwatch's lifecycle values in the state file. It serves
// startup, telemetry and the upgrade check.
type diskState struct {
	store  *storage.Store
	client kubernetes.Interface
}

// get decodes the state value under key into out.
func (d diskState) get(key string, out any) (bool, error) {
	return getRaw(storage.StateValues[json.RawMessage](d.store), key, out)
}

func (d diskState) put(key string, value any) error {
	return putRaw(storage.StateValues[json.RawMessage](d.store), key, value)
}

// getRaw decodes the value under key into out. A value that does not
// decode into out reads as absent, like a corrupt record.
func getRaw(
	values storage.Keyed[json.RawMessage], key string, out any,
) (bool, error) {
	raw, found, err := values.Get(key)
	if err != nil || !found {
		return false, err
	}
	if json.Unmarshal(raw, out) != nil {
		return false, nil
	}
	return true, nil
}

func putRaw(
	values storage.Keyed[json.RawMessage], key string, value any,
) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return values.Put(key, raw)
}

// EnsureClusterID returns the stored anonymous cluster ID, deriving a new
// one from the kube-system namespace UID on first start so a reinstall
// keeps the same identity.
func (d diskState) EnsureClusterID(ctx context.Context) (string, error) {
	var id string
	if found, err := d.get(stateClusterID, &id); err != nil || found {
		return id, err
	}
	id = uuid.New().String()
	if d.client != nil {
		ns, err := d.client.CoreV1().Namespaces().Get(ctx, "kube-system",
			metav1.GetOptions{})
		if err == nil && ns.UID != "" {
			id = clusterIDFromUID(string(ns.UID))
		}
	}
	return id, d.put(stateClusterID, id)
}

// clusterIDFromUID formats a salted SHA-256 of uid as a version 4 UUID;
// the UID itself never leaves the cluster.
func clusterIDFromUID(uid string) string {
	sum := sha256.Sum256([]byte("kwatch-cluster-id:" + uid))
	id, _ := uuid.FromBytes(sum[:16])
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return id.String()
}

// IsFirstRun implements startupStateStore.
func (d diskState) IsFirstRun(context.Context) (bool, error) {
	var done bool
	found, err := d.get(stateInitialized, &done)
	return !found || !done, err
}

// GetStoredVersion implements startupStateStore.
func (d diskState) GetStoredVersion(context.Context) (string, error) {
	var version string
	_, err := d.get(stateVersion, &version)
	return version, err
}

// MarkAsInitialized implements startupStateStore.
func (d diskState) MarkAsInitialized(
	_ context.Context, clusterID, version string,
) error {
	if err := d.put(stateClusterID, clusterID); err != nil {
		return err
	}
	if err := d.put(stateVersion, version); err != nil {
		return err
	}
	return d.put(stateInitialized, true)
}

// GetLastSeen implements startupStateStore.
func (d diskState) GetLastSeen(context.Context) (time.Time, error) {
	var at time.Time
	_, err := d.get(stateLastSeen, &at)
	return at, err
}

// SetLastSeen implements startupStateStore.
func (d diskState) SetLastSeen(_ context.Context, at time.Time) error {
	return d.put(stateLastSeen, at)
}

// GetRuntimeSession lets startup explain how the previous session ended.
func (d diskState) GetRuntimeSession(
	context.Context,
) (runtimeSession, error) {
	var session runtimeSession
	_, err := d.get(stateSession, &session)
	return session, err
}

// SaveRuntimeSession records the current session.
func (d diskState) SaveRuntimeSession(
	_ context.Context, session runtimeSession,
) error {
	return d.put(stateSession, session)
}

// ClaimStartupAnnouncement returns true once per announcement key, so a
// quick restart does not repeat the startup message.
func (d diskState) ClaimStartupAnnouncement(
	_ context.Context, key string,
) (bool, error) {
	var last string
	if _, err := d.get(stateAnnouncement, &last); err != nil {
		return false, err
	}
	if last == key {
		return false, nil
	}
	return true, d.put(stateAnnouncement, key)
}

// GetTelemetryLastSent implements the telemetry store.
func (d diskState) GetTelemetryLastSent(context.Context) (time.Time, error) {
	var at time.Time
	_, err := d.get(stateTelemetrySent, &at)
	return at, err
}

// SetTelemetryLastSent implements the telemetry store.
func (d diskState) SetTelemetryLastSent(
	_ context.Context, at time.Time,
) error {
	return d.put(stateTelemetrySent, at)
}

// GetNotifiedVersion implements the upgrader's version tracker.
func (d diskState) GetNotifiedVersion(context.Context) string {
	var version string
	_, _ = d.get(stateNotifiedUpdate, &version)
	return version
}

// SetNotifiedVersion implements the upgrader's version tracker.
func (d diskState) SetNotifiedVersion(
	_ context.Context, version string,
) error {
	return d.put(stateNotifiedUpdate, version)
}

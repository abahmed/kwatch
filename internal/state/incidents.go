package state

import (
	"context"
	"sort"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/model"
)

// ── Incident persistence ─────────────────────────────────────

func (s *StateManager) SaveIncidents(ctx context.Context, incidents any) error {
	return s.incidentsMgr.UpdateWithRetry(ctx, applyIncidents(incidents))
}

// SaveIncidentState writes the correlation snapshot to its dedicated
// ConfigMaps. Auxiliary state is written before the incident map is cleaned of
// legacy keys, so a failed upgrade can still recover the old snapshot.
func (s *StateManager) SaveIncidentState(
	ctx context.Context,
	incidents []model.PersistedIncident,
	groups []model.PersistedGroup,
	threads map[string]map[string]string,
	engine model.PersistedEngineState,
) error {
	if err := s.SavePersistedGroups(ctx, groups); err != nil {
		return err
	}
	if err := s.SaveProviderThreads(ctx, threads); err != nil {
		return err
	}
	if err := s.SaveEngineState(ctx, engine); err != nil {
		return err
	}
	return s.savePersistedIncidents(ctx, incidents, true)
}

func (s *StateManager) GetIncidents(ctx context.Context, out any) error {
	cm, err := s.client.CoreV1().ConfigMaps(
		s.namespace,
	).Get(
		ctx,
		incidentsConfigMapName,
		metav1.GetOptions{},
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil // nothing saved yet
		}
		return err
	}
	if gz, ok := cm.BinaryData[incidentsKey]; ok && len(gz) > 0 {
		return gunzipJSON(gz, out)
	}
	return nil
}

// SavePersistedIncidents and LoadPersistedIncidents are the typed contract for
// incident persistence.
//
// SaveIncidents/GetIncidents take `any`, and that is how the two sides of this
// format silently drifted apart: the writer stored a map[string]*model.Incident
// while the reader asked for a []model.PersistedIncident, so every restore
// failed and the only symptom was one log line. Nothing in the type system
// objected. Routing production code through these two functions puts the
// compiler back in charge of the contract; the untyped pair stays for the codec
// tests.
func (s *StateManager) SavePersistedIncidents(
	ctx context.Context,
	incidents []model.PersistedIncident,
) error {
	return s.savePersistedIncidents(ctx, incidents, false)
}

func (s *StateManager) savePersistedIncidents(
	ctx context.Context,
	incidents []model.PersistedIncident,
	cleanLegacy bool,
) error {
	return s.incidentsMgr.UpdateWithRetry(ctx, func(cm *corev1.ConfigMap) error {
		if err := applyIncidents(trimIncidentsToBudget(incidents))(cm); err != nil {
			return err
		}
		if cleanLegacy {
			// Remove auxiliary keys from the legacy combined ConfigMap only
			// after their dedicated writes have succeeded.
			deletePayload(cm, groupsKey)
			deletePayload(cm, threadsKey)
			deletePayload(cm, engineKey)
		}
		return nil
	})
}

// trimIncidentsToBudget keeps the largest deterministic prefix that fits the
// gzipped payload budget.
//
// The previous behaviour was all-or-nothing: one oversized snapshot meant
// nothing was written at all, so on a large cluster kwatch silently stopped
// persisting and lost every incident on each restart. Active incidents are
// more valuable than resolved history, then newer incidents win within each
// state. Partial memory beats none, and the prefix search avoids dropping more
// incidents than the limit requires.
func trimIncidentsToBudget(
	incidents []model.PersistedIncident,
) []model.PersistedIncident {
	if len(incidents) == 0 {
		return incidents
	}
	if data, err := gzJSON(
		incidents,
	); err == nil &&
		len(data) <= baselineMaxBytes {
		return incidents
	}

	// Active state must survive before resolved history. LastSeen breaks ties
	// within a state, and Key makes the result stable when timestamps match.
	trimmed := make([]model.PersistedIncident, len(incidents))
	copy(trimmed, incidents)
	sort.SliceStable(trimmed, func(i, j int) bool {
		left, right := trimmed[i], trimmed[j]
		leftPriority := incidentStatePriority(left.State)
		rightPriority := incidentStatePriority(right.State)
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		if !left.LastSeen.Equal(right.LastSeen) {
			return left.LastSeen.After(right.LastSeen)
		}
		return left.Key < right.Key
	})

	// Gzip size grows monotonically for this JSON prefix in practice. The
	// upper-biased search finds the maximum fitting prefix in O(log n) encodes.
	low, high := 0, len(trimmed)
	for low < high {
		middle := low + (high-low+1)/2
		data, err := gzJSON(trimmed[:middle])
		if err != nil {
			// Returning nil here would persist an empty list and erase every
			// incident. Hand back the input and let the save fail loudly.
			return incidents
		}
		if len(data) <= baselineMaxBytes {
			low = middle
			continue
		}
		high = middle - 1
	}
	if low == 0 {
		// A single incident can itself contain an unexpectedly large hint or
		// fact. Preserve the old value and let applyIncidents return an error
		// rather than replacing it with an empty snapshot.
		return incidents
	}

	klog.ErrorS(
		nil,
		"incident state exceeds the ConfigMap budget; keeping the most recent",
		"kept",
		low,
		"dropped",
		len(incidents)-low,
		"max",
		baselineMaxBytes,
	)
	return trimmed[:low]
}

func incidentStatePriority(state model.IncidentState) int {
	switch state {
	case model.StateActive:
		return 0
	case model.StatePendingResolve:
		return 1
	case model.StateResolved:
		return 2
	default:
		return 3
	}
}

// LoadPersistedIncidents reads incidents back, accepting the legacy object
// layout written by older versions. Without this an upgrade drops correlation
// memory and re-announces everything already broken as brand new.
func (s *StateManager) LoadPersistedIncidents(
	ctx context.Context,
) ([]model.PersistedIncident, error) {
	var incidents []model.PersistedIncident
	err := s.GetIncidents(ctx, &incidents)
	if err == nil {
		return incidents, nil
	}

	// Older releases stored an object keyed by incident id. Field names differ
	// only in case, which encoding/json matches, so the entries decode.
	var legacy map[string]model.PersistedIncident
	if legacyErr := s.GetIncidents(
		ctx,
		&legacy,
	); legacyErr != nil ||
		len(legacy) == 0 {
		return nil, err
	}

	keys := make([]string, 0, len(legacy))
	for k := range legacy {
		keys = append(keys, k)
	}
	// Map iteration order is random; keep restores deterministic.
	sort.Strings(keys)

	migrated := make([]model.PersistedIncident, 0, len(legacy))
	for _, k := range keys {
		migrated = append(migrated, legacy[k])
	}
	klog.InfoS("migrated incident state from the legacy object layout",
		"count", len(migrated))
	return migrated, nil
}

// SavePersistedGroups stores smart-group state next to the incidents, under
// its own ConfigMap entry.
func (s *StateManager) SavePersistedGroups(
	ctx context.Context,
	groups []model.PersistedGroup,
) error {
	return s.groupsMgr.UpdateWithRetry(ctx, applyGroups(groups))
}

// LoadPersistedGroups reads smart-group state. A missing entry is not an
// error: it is what an older release, or a first run, leaves behind.
func (s *StateManager) LoadPersistedGroups(
	ctx context.Context,
) ([]model.PersistedGroup, error) {
	var groups []model.PersistedGroup
	found, err := s.loadPayload(
		ctx, groupsConfigMapName, groupsKey, &groups,
	)
	if err != nil || found {
		return groups, err
	}
	if err := s.loadLegacyPayload(ctx, groupsKey, &groups); err != nil {
		return nil, err
	}
	return groups, nil
}

// SaveProviderThreads stores per-provider conversation ids (provider name →
// incident key → thread id) beside the incidents they belong to.
func (s *StateManager) SaveProviderThreads(
	ctx context.Context,
	threads map[string]map[string]string,
) error {
	return s.threadsMgr.UpdateWithRetry(ctx, applyThreads(threads))
}

// LoadProviderThreads reads saved conversation ids. A missing entry is what a
// first run, or a release without thread persistence, leaves behind.
func (s *StateManager) LoadProviderThreads(
	ctx context.Context,
) (map[string]map[string]string, error) {
	var threads map[string]map[string]string
	found, err := s.loadPayload(
		ctx, threadsConfigMapName, threadsKey, &threads,
	)
	if err != nil || found {
		return threads, err
	}
	if err := s.loadLegacyPayload(ctx, threadsKey, &threads); err != nil {
		return nil, err
	}
	return threads, nil
}

// SaveEngineState stores the correlation engine's remaining working memory
// beside the incidents it belongs to.
func (s *StateManager) SaveEngineState(
	ctx context.Context,
	engine model.PersistedEngineState,
) error {
	return s.engineMgr.UpdateWithRetry(ctx, applyEngineState(engine))
}

func isEmptyEngineState(engine model.PersistedEngineState) bool {
	return len(engine.Cooldowns) == 0 && len(engine.PodUIDs) == 0 &&
		len(engine.Containers) == 0 && len(engine.FanOut) == 0
}

// LoadEngineState reads the engine bookkeeping back. A missing entry is what
// a first run, or a release without it, leaves behind.
func (s *StateManager) LoadEngineState(
	ctx context.Context,
) (model.PersistedEngineState, error) {
	var engine model.PersistedEngineState
	found, err := s.loadPayload(
		ctx, engineConfigMapName, engineKey, &engine,
	)
	if err != nil || found {
		return engine, err
	}
	return engine, s.loadLegacyPayload(ctx, engineKey, &engine)
}

// loadPayload reports whether the dedicated ConfigMap exists. An existing map
// with no payload is authoritative, so an intentionally cleared new map never
// falls back to stale legacy data.
func (s *StateManager) loadPayload(
	ctx context.Context,
	name, key string,
	out any,
) (bool, error) {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(
		ctx, name, metav1.GetOptions{},
	)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	gz := cm.BinaryData[key]
	if len(gz) == 0 {
		return true, nil
	}
	return true, gunzipJSON(gz, out)
}

func (s *StateManager) loadLegacyPayload(
	ctx context.Context,
	key string,
	out any,
) error {
	found, err := s.loadPayload(ctx, incidentsConfigMapName, key, out)
	if !found || err != nil {
		return err
	}
	return nil
}

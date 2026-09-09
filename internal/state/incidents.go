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

// SaveIncidentState writes the whole correlation snapshot in one update.
//
// The four parts live in one ConfigMap but used to be written one at a time:
// four reads, four updates and four chances to conflict, every time anything
// changed. Worse, a failure partway through left the parts describing
// different moments -- group state naming incidents that were never written.
// One update is atomic and costs a quarter as much.
func (s *StateManager) SaveIncidentState(
	ctx context.Context,
	incidents []model.PersistedIncident,
	groups []model.PersistedGroup,
	threads map[string]map[string]string,
	engine model.PersistedEngineState,
) error {
	writers := []func(*corev1.ConfigMap) error{
		applyIncidents(trimIncidentsToBudget(incidents)),
		applyGroups(groups),
		applyThreads(threads),
		applyEngineState(engine),
	}
	return s.incidentsMgr.UpdateWithRetry(
		ctx,
		func(cm *corev1.ConfigMap) error {
			for _, write := range writers {
				if err := write(cm); err != nil {
					return err
				}
			}
			return nil
		},
	)
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
	return s.SaveIncidents(ctx, trimIncidentsToBudget(incidents))
}

// trimIncidentsToBudget drops the least recently seen incidents until the
// gzipped payload fits a ConfigMap.
//
// The previous behaviour was all-or-nothing: one oversized snapshot meant
// nothing was written at all, so on a large cluster kwatch silently stopped
// persisting and lost every incident on each restart. Keeping the freshest
// incidents that fit is strictly better — partial memory beats none, and the
// ones dropped are the stalest.
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

	// Freshest first, so truncation sheds the stalest.
	trimmed := make([]model.PersistedIncident, len(incidents))
	copy(trimmed, incidents)
	sort.SliceStable(trimmed, func(i, j int) bool {
		return trimmed[i].LastSeen.After(trimmed[j].LastSeen)
	})

	// Halve until it fits; a linear walk over thousands of incidents would
	// re-gzip thousands of times.
	n := len(trimmed)
	for n > 1 {
		n /= 2
		data, err := gzJSON(trimmed[:n])
		if err != nil {
			// Returning nil here would persist an empty list and erase every
			// incident. Hand back the input and let the save fail loudly.
			return incidents
		}
		if len(data) <= baselineMaxBytes {
			break
		}
	}
	klog.ErrorS(
		nil,
		"incident state exceeds the ConfigMap budget; keeping the most recent",
		"kept",
		n,
		"dropped",
		len(incidents)-n,
		"max",
		baselineMaxBytes,
	)
	return trimmed[:n]
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
	return s.incidentsMgr.UpdateWithRetry(ctx, applyGroups(groups))
}

// LoadPersistedGroups reads smart-group state. A missing entry is not an
// error: it is what an older release, or a first run, leaves behind.
func (s *StateManager) LoadPersistedGroups(
	ctx context.Context,
) ([]model.PersistedGroup, error) {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(
		ctx, incidentsConfigMapName, metav1.GetOptions{},
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	gz, ok := cm.BinaryData[groupsKey]
	if !ok || len(gz) == 0 {
		return nil, nil
	}
	var groups []model.PersistedGroup
	if err := gunzipJSON(gz, &groups); err != nil {
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
	return s.incidentsMgr.UpdateWithRetry(ctx, applyThreads(threads))
}

// LoadProviderThreads reads saved conversation ids. A missing entry is what a
// first run, or a release without thread persistence, leaves behind.
func (s *StateManager) LoadProviderThreads(
	ctx context.Context,
) (map[string]map[string]string, error) {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(
		ctx, incidentsConfigMapName, metav1.GetOptions{},
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	gz, ok := cm.BinaryData[threadsKey]
	if !ok || len(gz) == 0 {
		return nil, nil
	}
	var threads map[string]map[string]string
	if err := gunzipJSON(gz, &threads); err != nil {
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
	return s.incidentsMgr.UpdateWithRetry(ctx, applyEngineState(engine))
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
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(
		ctx, incidentsConfigMapName, metav1.GetOptions{},
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return engine, nil
		}
		return engine, err
	}
	gz, ok := cm.BinaryData[engineKey]
	if !ok || len(gz) == 0 {
		return engine, nil
	}
	if err := gunzipJSON(gz, &engine); err != nil {
		return engine, err
	}
	return engine, nil
}

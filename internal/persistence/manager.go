package persistence

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/metrics"
)

const (
	stateConfigMapName     = "kwatch-state"
	baselineConfigMapName  = "kwatch-baseline"
	incidentsConfigMapName = "kwatch-incidents"
	groupsConfigMapName    = "kwatch-groups"
	threadsConfigMapName   = "kwatch-threads"
	engineConfigMapName    = "kwatch-engine"
	pvcConfigMapName       = "kwatch-pvc"
	changesConfigMapName   = "kwatch-changes"
	rcaConfigMapName       = "kwatch-rca"
	telemetryConfigMapName = "kwatch-telemetry"
	initKey                = "kwatch-init"
	clusterIDKey           = "cluster-id"
	versionKey             = "version"
	stateSchemaVersionKey  = "state-schema-version"
	currentStateSchema     = "2"
	firstRunKey            = "first-run"
	notifiedVersionKey     = "notified-version"
	lastSeenKey            = "last-seen"
	telemetryLastSentKey   = "telemetry-last-sent"
	baselineKey            = "baseline"
	incidentsKey           = "incidents"
	// groupsKey is a separate entry so the incident payload keeps the exact
	// format older releases read: a version that does not know about groups
	// still restores incidents, and this one restores both.
	groupsKey   = "groups"
	threadsKey  = "threads"
	engineKey   = "engine"
	pvcUsageKey = "pvc-usage"
)

type Manager struct {
	client         kubernetes.Interface
	namespace      string
	configMapStore *RetryConfigMapManager // kwatch-state
	baselineMgr    *RetryConfigMapManager // kwatch-baseline
	incidentsMgr   *RetryConfigMapManager // kwatch-incidents
	groupsMgr      *RetryConfigMapManager // kwatch-groups
	threadsMgr     *RetryConfigMapManager // kwatch-threads
	engineMgr      *RetryConfigMapManager // kwatch-engine
	pvcMgr         *RetryConfigMapManager // kwatch-pvc
	changesMgr     *RetryConfigMapManager // kwatch-changes
	rcaMgr         *RetryConfigMapManager // kwatch-rca
	// telemetryMgr owns kwatch-telemetry. The kubelet monitor's per-cgroup
	// snapshot map is the largest thing kwatch persists and it grew with the
	// cluster; sharing kwatch-state meant one big cluster could push that
	// ConfigMap past its 1MB limit and fail every unrelated state write with
	// it.
	telemetryMgr    *RetryConfigMapManager // kwatch-telemetry
	now             func() time.Time
	migrationMu     sync.RWMutex
	migrationReport []MigrationResult
}

// NewManagerWithClock constructs the persistence manager with an explicit
// clock supplied by application composition.
func NewManagerWithClock(
	client kubernetes.Interface,
	namespace string,
	timeSource clock.Clock,
) *Manager {
	if timeSource == nil {
		timeSource = clock.RealClock{}
	}
	return &Manager{
		client:    client,
		namespace: namespace,
		configMapStore: NewRetryConfigMapManager(
			client,
			namespace,
			stateConfigMapName,
		),
		baselineMgr: NewRetryConfigMapManager(
			client,
			namespace,
			baselineConfigMapName,
		),
		incidentsMgr: NewRetryConfigMapManager(
			client,
			namespace,
			incidentsConfigMapName,
		),
		groupsMgr: NewRetryConfigMapManager(
			client,
			namespace,
			groupsConfigMapName,
		),
		threadsMgr: NewRetryConfigMapManager(
			client,
			namespace,
			threadsConfigMapName,
		),
		engineMgr: NewRetryConfigMapManager(
			client,
			namespace,
			engineConfigMapName,
		),
		pvcMgr: NewRetryConfigMapManager(
			client,
			namespace,
			pvcConfigMapName,
		),
		changesMgr: NewRetryConfigMapManager(
			client,
			namespace,
			changesConfigMapName,
		),
		rcaMgr: NewRetryConfigMapManager(
			client,
			namespace,
			rcaConfigMapName,
		),
		telemetryMgr: NewRetryConfigMapManager(
			client,
			namespace,
			telemetryConfigMapName,
		),
		now: timeSource.Now,
	}
}

func (s *Manager) nowTime() time.Time {
	return s.now()
}

func (s *Manager) SetLastSeen(ctx context.Context, t time.Time) error {
	return s.configMapStore.UpdateWithRetry(ctx, func(cm *corev1.ConfigMap) error {
		cm.Data[lastSeenKey] = t.UTC().Format(time.RFC3339)
		return nil
	})
}

// GetLastSeen returns when kwatch last recorded itself alive. The zero time
// means there is no record — a first run, or an install predating this key.
func (s *Manager) GetLastSeen(ctx context.Context) time.Time {
	cm, err := s.client.CoreV1().ConfigMaps(
		s.namespace,
	).Get(
		ctx,
		stateConfigMapName,
		metav1.GetOptions{},
	)
	if err != nil {
		return time.Time{}
	}
	raw, ok := cm.Data[lastSeenKey]
	if !ok || raw == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		klog.V(2).InfoS("ignoring unparsable last-seen value", "value", raw)
		return time.Time{}
	}
	return t
}

// GetTelemetryLastSent returns the last time the adoption heartbeat was
// successfully sent. The zero time means no heartbeat has been sent yet.
func (s *Manager) GetTelemetryLastSent(ctx context.Context) time.Time {
	cm, err := s.client.CoreV1().ConfigMaps(
		s.namespace,
	).Get(ctx, stateConfigMapName, metav1.GetOptions{})
	if err != nil {
		return time.Time{}
	}
	raw := cm.Data[telemetryLastSentKey]
	if raw == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		klog.V(2).InfoS("ignoring unparsable telemetry last-sent value", "value", raw)
		return time.Time{}
	}
	return t
}

// SetTelemetryLastSent records the last successful adoption heartbeat.
func (s *Manager) SetTelemetryLastSent(ctx context.Context, t time.Time) error {
	return s.configMapStore.UpdateWithRetry(ctx, func(cm *corev1.ConfigMap) error {
		if cm.Data == nil {
			cm.Data = make(map[string]string)
		}
		cm.Data[telemetryLastSentKey] = t.UTC().Format(time.RFC3339)
		return nil
	})
}

func (s *Manager) EnsureClusterID(ctx context.Context) (string, error) {
	clusterID, err := s.GetClusterID(ctx)
	if err == nil && clusterID != "" {
		return clusterID, nil
	}
	return uuid.New().String(), nil
}

func (s *Manager) MarkAsInitialized(
	ctx context.Context,
	clusterID, version string,
) error {
	s.resetMigrationReport()
	_, err := s.client.CoreV1().ConfigMaps(
		s.namespace,
	).Get(
		ctx,
		stateConfigMapName,
		metav1.GetOptions{},
	)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		cm := s.createConfigMap(clusterID, version)
		if _, err := s.client.CoreV1().ConfigMaps(
			s.namespace,
		).Create(
			ctx,
			cm,
			metav1.CreateOptions{},
		); err != nil {
			return err
		}
		klog.InfoS(
			"created state configmap with cluster ID",
			"clusterID",
			clusterID,
		)
		return nil
	}

	var migration MigrationResult
	err = s.configMapStore.UpdateWithRetry(ctx, func(c *corev1.ConfigMap) error {
		migration = migrateStateData(c.Data)
		if _, exists := c.Data[initKey]; !exists {
			c.Data[initKey] = "true"
		}
		if _, exists := c.Data[clusterIDKey]; !exists ||
			c.Data[clusterIDKey] == "" {
			c.Data[clusterIDKey] = clusterID
		}
		if _, exists := c.Data[firstRunKey]; !exists {
			c.Data[firstRunKey] = s.nowTime().UTC().Format(time.RFC3339)
		}
		c.Data[versionKey] = version
		return nil
	})
	if err != nil {
		s.recordMigrationResult(migration, err)
	}
	if err == nil {
		s.recordMigrationResult(migration, nil)
	}
	return err
}

// migrateStateData is deliberately conservative: state keys are additive and
// older installations can be upgraded by recording the current schema. A
// future schema is preserved so an older binary never silently downgrades it.
// Payload-specific migrations remain in their loaders (for example, the
// incident loader handles the legacy map format).
func migrateStateData(data map[string]string) MigrationResult {
	result := MigrationResult{
		Store:                 "state",
		SourceFormat:          "kwatch-state",
		DestinationFormat:     "kwatch-state/schema-v" + currentStateSchema,
		Status:                MigrationNotRequired,
		Recoverable:           true,
		MonitoringMayContinue: true,
		Detail:                "state schema is current",
	}
	if data == nil {
		result.Status = MigrationCompleted
		result.Detail = "state data was empty"
		return result
	}
	raw, exists := data[stateSchemaVersionKey]
	if !exists || raw == "" {
		data[stateSchemaVersionKey] = currentStateSchema
		result.Status = MigrationCompleted
		result.Detail = "recorded current state schema"
		return result
	}
	stored, err := strconv.Atoi(raw)
	if err != nil || stored < 1 {
		result.Status = MigrationFailed
		result.Recoverable = true
		result.Detail = "state schema version is malformed"
		return result
	}
	current, _ := strconv.Atoi(currentStateSchema)
	if stored > current {
		result.Status = MigrationUnsupported
		result.Detail = "state schema is newer than this binary supports"
		return result
	}
	if stored < current {
		data[stateSchemaVersionKey] = currentStateSchema
		result.Status = MigrationCompleted
		result.Detail = "upgraded state schema"
	}
	return result
}

// MigrationReport returns all migration outcomes recorded for the current
// startup cycle. The returned slice is independent of manager state.
func (s *Manager) MigrationReport() []MigrationResult {
	s.migrationMu.RLock()
	defer s.migrationMu.RUnlock()
	return append([]MigrationResult(nil), s.migrationReport...)
}

func (s *Manager) resetMigrationReport() {
	s.migrationMu.Lock()
	s.migrationReport = nil
	s.migrationMu.Unlock()
}

func (s *Manager) recordMigrationResult(
	result MigrationResult,
	err error,
) {
	if err != nil {
		result.Status = MigrationFailed
		if result.Detail == "" {
			result.Detail = "migration operation failed"
		}
	}
	s.migrationMu.Lock()
	s.migrationReport = append(s.migrationReport, result)
	s.migrationMu.Unlock()
	metrics.DefaultRegistry().PersistenceMigrations.Add(1)
	if result.Status == MigrationFailed ||
		result.Status == MigrationUnsupported {
		metrics.DefaultRegistry().PersistenceMigrationErr.Add(1)
	}
}

// Helpers for ConfigMap lifecycle and metadata.

func (s *Manager) createConfigMap(
	clusterID, version string,
) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      stateConfigMapName,
			Namespace: s.namespace,
		},
		Data: map[string]string{
			initKey:               "true",
			clusterIDKey:          clusterID,
			versionKey:            version,
			stateSchemaVersionKey: currentStateSchema,
			firstRunKey:           s.nowTime().UTC().Format(time.RFC3339),
		},
	}
}

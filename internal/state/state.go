package state

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/clock"
)

const (
	stateConfigMapName     = "kwatch-state"
	baselineConfigMapName  = "kwatch-baseline"
	incidentsConfigMapName = "kwatch-incidents"
	pvcConfigMapName       = "kwatch-pvc"
	changesConfigMapName   = "kwatch-changes"
	rcaConfigMapName       = "kwatch-rca"
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
	pvcUsageKey            = "pvc-usage"
)

// PvcSample is the persisted representation of a single PVC usage observation.
type PvcSample struct {
	Pct       float64 `json:"pct"`
	Namespace string  `json:"ns"`
	Name      string  `json:"name"`
	// PodName is the last mounting pod (for incident parity with the live
	// path).
	PodName string    `json:"pod"`
	Seen    time.Time `json:"seen"`
}

type StateManager struct {
	client       kubernetes.Interface
	namespace    string
	stateMgr     *RetryConfigMapManager // kwatch-state
	baselineMgr  *RetryConfigMapManager // kwatch-baseline
	incidentsMgr *RetryConfigMapManager // kwatch-incidents
	pvcMgr       *RetryConfigMapManager // kwatch-pvc
	changesMgr   *RetryConfigMapManager // kwatch-changes
	rcaMgr       *RetryConfigMapManager // kwatch-rca
	now          func() time.Time
}

func NewStateManager(
	client kubernetes.Interface,
	namespace string,
) *StateManager {
	return &StateManager{
		client:    client,
		namespace: namespace,
		stateMgr: NewRetryConfigMapManager(
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
		now: clock.Now,
	}
}

// SetClock injects the clock used for persisted lifecycle metadata.
func (s *StateManager) SetClock(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

func (s *StateManager) nowTime() time.Time {
	if s.now != nil {
		return s.now()
	}
	return clock.Now()
}

func (s *StateManager) IsFirstRun(ctx context.Context) (bool, error) {
	cm, err := s.client.CoreV1().ConfigMaps(
		s.namespace,
	).Get(
		ctx,
		stateConfigMapName,
		metav1.GetOptions{},
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
	_, exists := cm.Data[initKey]
	return !exists, nil
}

func (s *StateManager) GetClusterID(ctx context.Context) (string, error) {
	cm, err := s.client.CoreV1().ConfigMaps(
		s.namespace,
	).Get(
		ctx,
		stateConfigMapName,
		metav1.GetOptions{},
	)
	if err != nil {
		return "", err
	}
	return cm.Data[clusterIDKey], nil
}

func (s *StateManager) GetStoredVersion(ctx context.Context) string {
	cm, err := s.client.CoreV1().ConfigMaps(
		s.namespace,
	).Get(
		ctx,
		stateConfigMapName,
		metav1.GetOptions{},
	)
	if err != nil {
		return ""
	}
	return cm.Data[versionKey]
}

// GetStateSchemaVersion reports the persisted state format. An empty value is
// a legacy installation that predates explicit schema tracking.
func (s *StateManager) GetStateSchemaVersion(ctx context.Context) string {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, stateConfigMapName, metav1.GetOptions{})
	if err != nil {
		return ""
	}
	return cm.Data[stateSchemaVersionKey]
}

func (s *StateManager) GetNotifiedVersion(ctx context.Context) string {
	cm, err := s.client.CoreV1().ConfigMaps(
		s.namespace,
	).Get(
		ctx,
		stateConfigMapName,
		metav1.GetOptions{},
	)
	if err != nil {
		return ""
	}
	return cm.Data[notifiedVersionKey]
}

func (s *StateManager) SetNotifiedVersion(
	ctx context.Context,
	version string,
) error {
	return s.stateMgr.UpdateWithRetry(ctx, func(cm *corev1.ConfigMap) error {
		cm.Data[notifiedVersionKey] = version
		return nil
	})
}

// ── liveness gap ──────────────────────────────────────────────

// SetLastSeen records that kwatch was alive at t. Written periodically so a
// later start can tell how long monitoring was actually down.
func (s *StateManager) SetLastSeen(ctx context.Context, t time.Time) error {
	return s.stateMgr.UpdateWithRetry(ctx, func(cm *corev1.ConfigMap) error {
		cm.Data[lastSeenKey] = t.UTC().Format(time.RFC3339)
		return nil
	})
}

// GetLastSeen returns when kwatch last recorded itself alive. The zero time
// means there is no record — a first run, or an install predating this key.
func (s *StateManager) GetLastSeen(ctx context.Context) time.Time {
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

// GetTelemetryLastSent returns the last time the anonymous heartbeat was
// successfully sent. The zero time means no heartbeat has been sent yet.
func (s *StateManager) GetTelemetryLastSent(ctx context.Context) time.Time {
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

// SetTelemetryLastSent records the last successful anonymous heartbeat.
func (s *StateManager) SetTelemetryLastSent(ctx context.Context, t time.Time) error {
	return s.stateMgr.UpdateWithRetry(ctx, func(cm *corev1.ConfigMap) error {
		if cm.Data == nil {
			cm.Data = make(map[string]string)
		}
		cm.Data[telemetryLastSentKey] = t.UTC().Format(time.RFC3339)
		return nil
	})
}

func (s *StateManager) EnsureClusterID(ctx context.Context) (string, error) {
	clusterID, err := s.GetClusterID(ctx)
	if err == nil && clusterID != "" {
		return clusterID, nil
	}
	return uuid.New().String(), nil
}

func (s *StateManager) MarkAsInitialized(
	ctx context.Context,
	clusterID, version string,
) error {
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

	return s.stateMgr.UpdateWithRetry(ctx, func(c *corev1.ConfigMap) error {
		migrateStateData(c.Data)
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
}

// migrateStateData is deliberately conservative: state keys are additive and
// older installations can be upgraded by recording the current schema. A
// future schema is preserved so an older binary never silently downgrades it.
// Payload-specific migrations remain in their loaders (for example, the
// incident loader handles the legacy map format).
func migrateStateData(data map[string]string) {
	if data == nil {
		return
	}
	stored, err := strconv.Atoi(data[stateSchemaVersionKey])
	if err != nil {
		if data[stateSchemaVersionKey] == "" || stored < 1 {
			data[stateSchemaVersionKey] = currentStateSchema
		}
		return
	}
	current, _ := strconv.Atoi(currentStateSchema)
	if stored < current {
		data[stateSchemaVersionKey] = currentStateSchema
	}
}

// ── helpers ───────────────────────────────────────────────────

func (s *StateManager) createConfigMap(
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

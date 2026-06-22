package state

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

const (
	stateConfigMapName    = "kwatch-state"
	baselineConfigMapName = "kwatch-baseline"
	pvcConfigMapName      = "kwatch-pvc"
	initKey               = "kwatch-init"
	clusterIDKey          = "cluster-id"
	versionKey            = "version"
	firstRunKey           = "first-run"
	notifiedVersionKey    = "notified-version"
	baselineKey           = "baseline"
	pvcUsageKey           = "pvc-usage"
)

// PvcSample is the persisted representation of a single PVC usage observation.
type PvcSample struct {
	Pct       float64   `json:"pct"`
	Namespace string    `json:"ns"`
	Name      string    `json:"name"`
	PodName   string    `json:"pod"` // last mounting pod (for incident parity with the live path)
	Seen      time.Time `json:"seen"`
}

type StateManager struct {
	client      kubernetes.Interface
	namespace   string
	stateMgr    *RetryConfigMapManager // kwatch-state
	baselineMgr *RetryConfigMapManager // kwatch-baseline
	pvcMgr      *RetryConfigMapManager // kwatch-pvc
}

func NewStateManager(client kubernetes.Interface, namespace string) *StateManager {
	return &StateManager{
		client:      client,
		namespace:   namespace,
		stateMgr:    NewRetryConfigMapManager(client, namespace, stateConfigMapName),
		baselineMgr: NewRetryConfigMapManager(client, namespace, baselineConfigMapName),
		pvcMgr:      NewRetryConfigMapManager(client, namespace, pvcConfigMapName),
	}
}

func (s *StateManager) IsFirstRun(ctx context.Context) (bool, error) {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, stateConfigMapName, metav1.GetOptions{})
	if err != nil {
		return true, nil
	}
	_, exists := cm.Data[initKey]
	return !exists, nil
}

func (s *StateManager) GetClusterID(ctx context.Context) (string, error) {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, stateConfigMapName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	return cm.Data[clusterIDKey], nil
}

func (s *StateManager) GetStoredVersion(ctx context.Context) string {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, stateConfigMapName, metav1.GetOptions{})
	if err != nil {
		return ""
	}
	return cm.Data[versionKey]
}

func (s *StateManager) GetNotifiedVersion(ctx context.Context) string {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, stateConfigMapName, metav1.GetOptions{})
	if err != nil {
		return ""
	}
	return cm.Data[notifiedVersionKey]
}

func (s *StateManager) SetNotifiedVersion(ctx context.Context, version string) error {
	return s.stateMgr.UpdateWithRetry(ctx, func(cm *corev1.ConfigMap) error {
		cm.Data[notifiedVersionKey] = version
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

func (s *StateManager) MarkAsInitialized(ctx context.Context, clusterID, version string) error {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, stateConfigMapName, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		cm = s.createConfigMap(clusterID, version)
		_, err = s.client.CoreV1().ConfigMaps(s.namespace).Create(ctx, cm, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		klog.InfoS("created state configmap with cluster ID", "clusterID", clusterID)
		return nil
	}

	return s.stateMgr.UpdateWithRetry(ctx, func(c *corev1.ConfigMap) error {
		if _, exists := c.Data[initKey]; !exists {
			c.Data[initKey] = "true"
		}
		if _, exists := c.Data[clusterIDKey]; !exists || c.Data[clusterIDKey] == "" {
			c.Data[clusterIDKey] = clusterID
		}
		if _, exists := c.Data[firstRunKey]; !exists {
			c.Data[firstRunKey] = time.Now().UTC().Format(time.RFC3339)
		}
		c.Data[versionKey] = version
		return nil
	})
}

// ── gzip helpers (shared by baseline and pvc-usage) ────────────

func gzJSON(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(raw); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func gunzipJSON(b []byte, out any) error {
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer zr.Close()
	raw, err := io.ReadAll(zr)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

// ── Baseline persistence ──────────────────────────────────────

const configMapDataLimit = 1 << 20                    // 1,048,576 — K8s ConfigMap data hard cap (MaxSecretSize)
const baselineMaxBytes = configMapDataLimit - 16*1024 // ~1,032,192; 16 KiB reserve for safety

func (s *StateManager) GetBaseline(ctx context.Context) map[string]map[string]int64 {
	var result map[string]map[string]int64

	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, baselineConfigMapName, metav1.GetOptions{})
	if err == nil {
		if gz, ok := cm.BinaryData[baselineKey]; ok && len(gz) > 0 {
			if err := gunzipJSON(gz, &result); err != nil {
				klog.ErrorS(err, "failed to gunzip baseline")
				return nil
			}
			return result
		}
		if raw, ok := cm.Data[baselineKey]; ok && raw != "" {
			if err := json.Unmarshal([]byte(raw), &result); err != nil {
				klog.ErrorS(err, "failed to unmarshal baseline")
				return nil
			}
			return result
		}
	}

	// migration: fall back to the pre-split location kwatch-state.data[baseline]
	if old, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, stateConfigMapName, metav1.GetOptions{}); err == nil {
		if raw, ok := old.Data[baselineKey]; ok && raw != "" {
			if err := json.Unmarshal([]byte(raw), &result); err != nil {
				klog.ErrorS(err, "failed to unmarshal legacy baseline")
				return nil
			}
			return result
		}
	}

	return nil
}

func (s *StateManager) SaveBaseline(ctx context.Context, baseline map[string]map[string]int64) error {
	return s.baselineMgr.UpdateWithRetry(ctx, func(cm *corev1.ConfigMap) error {
		data, err := gzJSON(baseline)
		if err != nil {
			return err
		}
		if len(data) > baselineMaxBytes {
			klog.ErrorS(nil, "baseline too large even gzipped, skipping save",
				"size", len(data), "max", baselineMaxBytes)
			return fmt.Errorf("baseline %d gz-bytes exceeds budget %d", len(data), baselineMaxBytes)
		}
		if cm.BinaryData == nil {
			cm.BinaryData = map[string][]byte{}
		}
		cm.BinaryData[baselineKey] = data
		delete(cm.Data, baselineKey)
		return nil
	})
}

// ── PVC usage persistence ─────────────────────────────────────

func (s *StateManager) GetPvcUsage(ctx context.Context) map[string]PvcSample {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, pvcConfigMapName, metav1.GetOptions{})
	if err != nil {
		return nil
	}
	if gz, ok := cm.BinaryData[pvcUsageKey]; ok && len(gz) > 0 {
		var result map[string]PvcSample
		if err := gunzipJSON(gz, &result); err != nil {
			klog.ErrorS(err, "failed to gunzip pvc usage")
			return nil
		}
		return result
	}
	raw, ok := cm.Data[pvcUsageKey]
	if !ok || raw == "" {
		return nil
	}
	var result map[string]PvcSample
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		klog.ErrorS(err, "failed to unmarshal pvc usage")
		return nil
	}
	return result
}

func (s *StateManager) SavePvcUsage(ctx context.Context, usage map[string]PvcSample) error {
	return s.pvcMgr.UpdateWithRetry(ctx, func(cm *corev1.ConfigMap) error {
		data, err := gzJSON(usage)
		if err != nil {
			return err
		}
		if len(data) > baselineMaxBytes {
			klog.ErrorS(nil, "pvc usage too large for ConfigMap, skipping save",
				"size", len(data), "max", baselineMaxBytes)
			return fmt.Errorf("pvc usage %d bytes exceeds ConfigMap budget %d", len(data), baselineMaxBytes)
		}
		if cm.BinaryData == nil {
			cm.BinaryData = map[string][]byte{}
		}
		cm.BinaryData[pvcUsageKey] = data
		delete(cm.Data, pvcUsageKey)
		return nil
	})
}

// ── Legacy baseline migration ─────────────────────────────────

// MigrateLegacyBaseline moves baseline data from kwatch-state.data[baseline]
// to the dedicated kwatch-baseline ConfigMap, then clears the legacy key.
// Idempotent — safe to call every startup.
func (s *StateManager) MigrateLegacyBaseline(ctx context.Context) {
	// Already in the dedicated CM? nothing to migrate.
	if cm, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, baselineConfigMapName, metav1.GetOptions{}); err == nil {
		if len(cm.BinaryData[baselineKey]) > 0 || cm.Data[baselineKey] != "" {
			s.clearLegacyBaseline(ctx)
			return
		}
	}
	old, err := s.client.CoreV1().ConfigMaps(s.namespace).Get(ctx, stateConfigMapName, metav1.GetOptions{})
	if err != nil {
		return
	}
	raw, ok := old.Data[baselineKey]
	if !ok || raw == "" {
		return
	}
	var b map[string]map[string]int64
	if err := json.Unmarshal([]byte(raw), &b); err != nil {
		klog.ErrorS(err, "migrate: bad legacy baseline json")
		return
	}
	if err := s.SaveBaseline(ctx, b); err != nil {
		klog.ErrorS(err, "migrate: save baseline to dedicated CM")
		return
	}
	s.clearLegacyBaseline(ctx)
}

func (s *StateManager) clearLegacyBaseline(ctx context.Context) {
	if err := s.stateMgr.UpdateWithRetry(ctx, func(cm *corev1.ConfigMap) error {
		delete(cm.Data, baselineKey)
		return nil
	}); err != nil {
		klog.ErrorS(err, "migrate: clear legacy baseline from kwatch-state")
	}
}

// ── helpers ───────────────────────────────────────────────────

func (s *StateManager) createConfigMap(clusterID, version string) *corev1.ConfigMap {
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
			initKey:      "true",
			clusterIDKey: clusterID,
			versionKey:   version,
			firstRunKey:  time.Now().UTC().Format(time.RFC3339),
		},
	}
}

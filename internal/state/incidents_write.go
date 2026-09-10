package state

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/model"
)

// The writers below stage one payload into its dedicated ConfigMap. They are
// plain functions so each Save* method shares the same empty/delete behavior.

// applyIncidents stores the incident list. Unlike the other three parts an
// oversized payload is an error: the caller must not go on to write state
// that describes incidents nobody saved.
func applyIncidents(incidents any) func(*corev1.ConfigMap) error {
	return func(cm *corev1.ConfigMap) error {
		data, err := gzJSON(incidents)
		if err != nil {
			return err
		}
		if len(data) > baselineMaxBytes {
			klog.ErrorS(
				nil,
				"incidents too large for ConfigMap, skipping save",
				"size",
				len(data),
				"max",
				baselineMaxBytes,
			)
			return fmt.Errorf(
				"incidents %d bytes exceeds ConfigMap budget %d",
				len(data),
				baselineMaxBytes,
			)
		}
		setBinaryPayload(cm, incidentsKey, data)
		return nil
	}
}

func applyGroups(
	groups []model.PersistedGroup,
) func(*corev1.ConfigMap) error {
	return func(cm *corev1.ConfigMap) error {
		if len(groups) == 0 {
			deletePayload(cm, groupsKey)
			return nil
		}
		data, err := gzJSON(groups)
		if err != nil {
			return err
		}
		if len(data) > baselineMaxBytes {
			// Groups are recoverable state: losing them costs duplicate
			// resolves, not correctness, so a payload that does not fit is
			// dropped rather than failing the incident save with it.
			klog.ErrorS(nil, "group state too large for ConfigMap, skipping",
				"size", len(data), "max", baselineMaxBytes)
			deletePayload(cm, groupsKey)
			return nil
		}
		setBinaryPayload(cm, groupsKey, data)
		if err := validateConfigMapData(cm); err != nil {
			klog.ErrorS(err, "group ConfigMap data exceeds budget, skipping")
			deletePayload(cm, groupsKey)
		}
		return nil
	}
}

func applyThreads(
	threads map[string]map[string]string,
) func(*corev1.ConfigMap) error {
	return func(cm *corev1.ConfigMap) error {
		if len(threads) == 0 {
			deletePayload(cm, threadsKey)
			return nil
		}
		data, err := gzJSON(threads)
		if err != nil {
			return err
		}
		if len(data) > baselineMaxBytes {
			// Thread ids are a presentation nicety: losing them costs a
			// resolve posted at top level, so an oversized payload is
			// dropped rather than failing the save it travels with.
			klog.ErrorS(nil, "thread state too large for ConfigMap, skipping",
				"size", len(data), "max", baselineMaxBytes)
			deletePayload(cm, threadsKey)
			return nil
		}
		setBinaryPayload(cm, threadsKey, data)
		if err := validateConfigMapData(cm); err != nil {
			klog.ErrorS(err, "thread ConfigMap data exceeds budget, skipping")
			deletePayload(cm, threadsKey)
		}
		return nil
	}
}

func applyEngineState(
	engine model.PersistedEngineState,
) func(*corev1.ConfigMap) error {
	return func(cm *corev1.ConfigMap) error {
		if isEmptyEngineState(engine) {
			deletePayload(cm, engineKey)
			return nil
		}
		data, err := gzJSON(engine)
		if err != nil {
			return err
		}
		if len(data) > baselineMaxBytes {
			// Recoverable state: losing it costs a louder first alert
			// after a restart, not correctness, so an oversized payload
			// is dropped rather than failing the incident save with it.
			klog.ErrorS(nil, "engine state too large for ConfigMap, skipping",
				"size", len(data), "max", baselineMaxBytes)
			deletePayload(cm, engineKey)
			return nil
		}
		setBinaryPayload(cm, engineKey, data)
		if err := validateConfigMapData(cm); err != nil {
			klog.ErrorS(err, "engine ConfigMap data exceeds budget, skipping")
			deletePayload(cm, engineKey)
		}
		return nil
	}
}

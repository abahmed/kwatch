package state

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/model"
)

// The writers below each stage one part of the correlation snapshot into the
// incidents ConfigMap. They are plain functions rather than methods so
// SaveIncidentState can apply all four inside a single update while each
// single-part Save* still applies only its own.

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
		if cm.BinaryData == nil {
			cm.BinaryData = map[string][]byte{}
		}
		cm.BinaryData[incidentsKey] = data
		return nil
	}
}

func applyGroups(
	groups []model.PersistedGroup,
) func(*corev1.ConfigMap) error {
	return func(cm *corev1.ConfigMap) error {
		if len(groups) == 0 {
			delete(cm.BinaryData, groupsKey)
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
			delete(cm.BinaryData, groupsKey)
			return nil
		}
		if cm.BinaryData == nil {
			cm.BinaryData = map[string][]byte{}
		}
		cm.BinaryData[groupsKey] = data
		return nil
	}
}

func applyThreads(
	threads map[string]map[string]string,
) func(*corev1.ConfigMap) error {
	return func(cm *corev1.ConfigMap) error {
		if len(threads) == 0 {
			delete(cm.BinaryData, threadsKey)
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
			delete(cm.BinaryData, threadsKey)
			return nil
		}
		if cm.BinaryData == nil {
			cm.BinaryData = map[string][]byte{}
		}
		cm.BinaryData[threadsKey] = data
		return nil
	}
}

func applyEngineState(
	engine model.PersistedEngineState,
) func(*corev1.ConfigMap) error {
	return func(cm *corev1.ConfigMap) error {
		if isEmptyEngineState(engine) {
			delete(cm.BinaryData, engineKey)
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
			delete(cm.BinaryData, engineKey)
			return nil
		}
		if cm.BinaryData == nil {
			cm.BinaryData = map[string][]byte{}
		}
		cm.BinaryData[engineKey] = data
		return nil
	}
}

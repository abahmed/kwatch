package persistence

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

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
		if len(data) > configMapPayloadMaxBytes {
			klog.ErrorS(
				nil,
				"incidents too large for ConfigMap, skipping save",
				"size",
				len(data),
				"max",
				configMapPayloadMaxBytes,
			)
			return fmt.Errorf(
				"incidents %d bytes exceeds ConfigMap budget %d",
				len(data),
				configMapPayloadMaxBytes,
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
		if len(data) > configMapPayloadMaxBytes {
			// Groups are recoverable state: losing them costs duplicate
			// resolves, not correctness, so a payload that does not fit is
			// dropped rather than failing the incident save with it.
			klog.ErrorS(nil, "group state too large for ConfigMap, skipping",
				"size", len(data), "max", configMapPayloadMaxBytes)
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
		if len(data) > configMapPayloadMaxBytes {
			// Thread ids are a presentation nicety: losing one costs an
			// update posted at top level. Drop the oldest threads until the
			// payload fits rather than losing every thread at once.
			data, err = trimThreads(threads)
			if err != nil {
				return err
			}
		}
		if len(data) > configMapPayloadMaxBytes {
			klog.ErrorS(nil, "thread state too large for ConfigMap, skipping",
				"size", len(data), "max", configMapPayloadMaxBytes)
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
		if len(data) > configMapPayloadMaxBytes {
			// Recoverable state: losing it costs a louder first alert
			// after a restart, not correctness, so an oversized payload
			// is dropped rather than failing the incident save with it.
			klog.ErrorS(nil, "engine state too large for ConfigMap, skipping",
				"size", len(data), "max", configMapPayloadMaxBytes)
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

type threadEntry struct {
	provider, key, ts string
}

// trimThreads removes the oldest tenth of thread entries, by thread
// timestamp, until the encoded payload fits. The input is not modified.
func trimThreads(threads map[string]map[string]string) ([]byte, error) {
	entries := make([]threadEntry, 0)
	for provider, keys := range threads {
		for key, value := range keys {
			entries = append(entries, threadEntry{
				provider: provider, key: key, ts: threadTimestamp(value),
			})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].ts != entries[j].ts {
			return entries[i].ts < entries[j].ts
		}
		return entries[i].provider+entries[i].key <
			entries[j].provider+entries[j].key
	})
	for len(entries) > 0 {
		entries = entries[max(1, len(entries)/10):]
		kept := make(map[string]map[string]string)
		for _, e := range entries {
			if kept[e.provider] == nil {
				kept[e.provider] = make(map[string]string)
			}
			kept[e.provider][e.key] = threads[e.provider][e.key]
		}
		data, err := gzJSON(kept)
		if err != nil {
			return nil, err
		}
		if len(data) <= configMapPayloadMaxBytes {
			klog.InfoS("trimmed oldest thread state to fit ConfigMap",
				"kept", len(entries))
			return data, nil
		}
	}
	return gzJSON(map[string]map[string]string{})
}

// threadTimestamp reads the sortable Slack timestamp from a plain thread id
// or from an encoded conversation state.
func threadTimestamp(value string) string {
	var state struct{ ThreadTS string }
	if strings.HasPrefix(value, "{") &&
		json.Unmarshal([]byte(value), &state) == nil {
		return state.ThreadTS
	}
	return value
}

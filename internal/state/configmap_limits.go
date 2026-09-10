package state

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

// configMapDataSize matches the apiserver's ConfigMap data accounting: every
// data key and value contributes to the limit, including BinaryData values.
// Object metadata and the serialized JSON/protobuf envelope are not part of
// this data limit, so the writers keep a separate safety reserve as well.
func configMapDataSize(cm *corev1.ConfigMap) int64 {
	var size int64
	for key, value := range cm.Data {
		size += int64(len(key)) + int64(len(value))
	}
	for key, value := range cm.BinaryData {
		size += int64(len(key)) + int64(len(value))
	}
	return size
}

func validateConfigMapData(cm *corev1.ConfigMap) error {
	size := configMapDataSize(cm)
	if size <= int64(baselineMaxBytes) {
		return nil
	}
	return fmt.Errorf(
		"configmap data %d bytes exceeds budget %d",
		size,
		baselineMaxBytes,
	)
}

// setBinaryPayload replaces a payload regardless of which field an older
// release used. This also prevents a stale Data key from being counted twice
// or rejected by ConfigMap validation as an overlapping key.
func setBinaryPayload(cm *corev1.ConfigMap, key string, value []byte) {
	if cm.BinaryData == nil {
		cm.BinaryData = make(map[string][]byte)
	}
	cm.BinaryData[key] = value
	delete(cm.Data, key)
}

func setStringPayload(cm *corev1.ConfigMap, key, value string) {
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	cm.Data[key] = value
	delete(cm.BinaryData, key)
}

func deletePayload(cm *corev1.ConfigMap, key string) {
	delete(cm.Data, key)
	delete(cm.BinaryData, key)
}

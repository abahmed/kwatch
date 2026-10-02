package kubeclient

import "os"

// GetNamespace returns the namespace where kwatch is running.
// It reads from POD_NAMESPACE environment variable and falls back to "kwatch".
func GetNamespace() string {
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		return "kwatch"
	}
	return namespace
}

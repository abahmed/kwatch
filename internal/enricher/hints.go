package enricher

var defaultHints = map[string]string{
	"OOMKilled":        "Memory pressure — consider increasing memory limits",
	"ImagePullBackOff": "Registry or authentication issue — check image name and pull secret",
	"CrashLoopBackOff": "Application crash — check logs for startup errors",
	"Error":            "Container exited with error — check logs",
	"NodeNotReady":     "Node not ready — check kubelet and node resources",
	"Unschedulable":    "No available node — check cluster capacity and resource requests",
}

func hintForReason(reason string) string {
	if h, ok := defaultHints[reason]; ok {
		return h
	}
	return ""
}

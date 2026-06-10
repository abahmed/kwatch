package event

// Event used to represent info needed by providers to send messages
type Event struct {
	Resource      string            // "pod", "node", "pvc"
	PodName       string
	ContainerName string
	Namespace     string
	NodeName      string
	Reason        string
	Events        string
	Logs          string
	Labels        map[string]string
	OwnerKind     string
	RestartCount  int
}

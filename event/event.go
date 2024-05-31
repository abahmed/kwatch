package event

// Event used to represent info needed by providers to send messages
type Event struct {
	PodName       string
	ContainerName string
	Namespace     string
	Reason        string
	Events        string
	Logs          string
	Labels        map[string]string
}

package detector

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"time"
)

// Input represents data flowing through the pipeline
type Input struct {
	// Source data
	Pod       *corev1.Pod
	Node      *corev1.Node
	Container *corev1.ContainerStatus
	Events    *[]corev1.Event
	Owner     *metav1.OwnerReference
	EventType string

	// Detection results
	HasIssue         bool
	IssueType        string // "pod" | "container" | "node"
	Reason           string
	Message          string
	ExitCode         int32
	Logs             string
	Status           string
	RestartCount     int32
	LastTerminatedOn time.Time

	// Dependencies
	Client kubernetes.Interface
	Config interface{}
	Volume Volume
}

// Event represents the output from pipeline
type Event struct {
	Type      string
	Name      string
	Container string
	Namespace string
	Node      string
	Reason    string
	Message   string
	Events    string
	Logs      string
	Labels    map[string]string
}

// Predicate filters events before processing (like k8s predicate)
type Predicate interface {
	Name() string
	Filter(input *Input) bool
}

// Detector finds issues in events
type Detector interface {
	Name() string
	Detect(input *Input) bool
}

// Handler enriches or transforms events (like k8s handler)
type Handler interface {
	Name() string
	Handle(input *Input) error
}

// Volume provides persistent storage interface
type Volume interface {
	Read(key string) ([]byte, error)
	Write(key string, data []byte) error
	Delete(key string) error
}

// Deduplication prevents duplicate alerts
type Deduplication interface {
	Name() string
	ShouldAlert(input *Input) bool
	Record(input *Input)
}

// Aggregator groups repeated events
type Aggregator interface {
	Name() string
	Process(input *Input) *Event
	ShouldAggregate(input *Input) bool
}

// ClusterDetector detects patterns across cluster
type ClusterDetector interface {
	Name() string
	Detect(input *Input) *Event
}

// Pipeline orchestrates the flow
type Pipeline interface {
	ProcessPod(input *Input) *Event
	ProcessNode(input *Input) *Event
}

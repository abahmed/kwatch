package statuswatch

import (
	"context"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	corev1lister "k8s.io/client-go/listers/core/v1"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/k8s/dynamicwatch"
	"github.com/abahmed/kwatch/internal/monitor"
)

// Monitor watches APIService and discovered CRD instances. It only treats
// well-known failure-shaped conditions as incidents; arbitrary informational
// status fields are deliberately ignored to prevent operator noise.
type Monitor struct {
	client            dynamic.Interface
	discoveryClient   discovery.DiscoveryInterfaceWithContext
	incidentSink      monitor.ObservationSink
	resync            time.Duration
	ctx               context.Context
	cancel            context.CancelFunc
	done              chan struct{}
	namespaceAllowed  func(string) bool
	namespaces        []string
	watchAll          bool
	mu                sync.Mutex
	lifecycleMu       sync.Mutex
	started           bool
	resetting         bool
	configured        bool
	generation        uint64
	runWG             *sync.WaitGroup
	factories         map[string]dynamicwatch.Factory
	stops             map[string]context.CancelFunc
	versionDone       map[string]chan struct{}
	staticWatcher     *dynamicwatch.Watcher
	staticGeneration  dynamicwatch.Generation
	crdVersions       map[string]map[string]struct{}
	conditionRules    map[string]map[string]bool
	graph             *kwcontext.ResourceGraph
	graphReferences   []graphReferenceRule
	admissionPolicies map[string]struct{}
	admissionBindings map[string]*unstructured.Unstructured
	// serviceLister answers the Service lookup a legacy Endpoints object
	// needs, from the informer cache instead of a live API read.
	serviceLister corev1lister.ServiceLister
	now           func() time.Time
}

type graphReferenceRule struct {
	path []string
	kind string
}

// cacheSyncTimeout bounds the initial informer sync, matching the
// controller's own bound. A var so tests can shorten it.
var cacheSyncTimeout = 5 * time.Minute

package pvc

import (
	"context"
	"sort"
	"sync"
	"time"

	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/monitor"
)

const (
	maxConcurrentSamples = 10
	maxConcurrentNodeOps = 5
)

type PvcMonitor struct {
	client         kubernetes.Interface
	config         config.PvcMonitor
	incidentSink   monitor.ObservationSink
	state          StateStore
	notifiedPvc    map[string]bool
	lastUsage      map[string]model.PVCSample
	pvByPVC        map[string]string
	pvByPVCAt      time.Time
	now            func() time.Time
	mu             sync.RWMutex
	firstScan      bool
	sem            chan struct{}
	getNodeUsageFn func(
		ctx context.Context,
		nodeName string,
		pvByPVC map[string]string,
	) ([]*PvcUsage, error)
	allowedNamespaces   map[string]struct{}
	forbiddenNamespaces map[string]struct{}
	namespaceFilter     func(string) bool
	watchAll            bool
	configured          bool
	started             bool
}

// StateStore is the only persistence capability required by the PVC monitor.
type StateStore interface {
	GetPvcUsage(context.Context) map[string]model.PVCSample
	SavePvcUsage(context.Context, map[string]model.PVCSample) error
}

// NewPvcMonitorWithRuntimeAndClock builds the monitor from the immutable
// runtime snapshot and an explicit application clock.
func NewPvcMonitorWithRuntimeAndClock(
	client kubernetes.Interface,
	runtime config.RuntimeConfig,
	incidentSink monitor.ObservationSink,
	stateStore StateStore,
	timeSource clock.Clock,
) *PvcMonitor {
	monitorConfig := runtime.PvcMonitor()
	if timeSource == nil {
		timeSource = clock.RealClock{}
	}
	return newPvcMonitor(
		client, monitorConfig, incidentSink, stateStore, timeSource.Now,
	)
}

func newPvcMonitor(
	client kubernetes.Interface,
	monitorConfig config.PvcMonitor,
	incidentSink monitor.ObservationSink,
	stateStore StateStore,
	now func() time.Time,
) *PvcMonitor {
	if now == nil {
		now = clock.RealClock{}.Now
	}
	return &PvcMonitor{
		client:       client,
		config:       monitorConfig,
		incidentSink: incidentSink,
		state:        stateStore,
		notifiedPvc:  make(map[string]bool),
		lastUsage:    make(map[string]model.PVCSample),
		now:          now,
		firstScan:    true,
		watchAll:     true,
		sem:          make(chan struct{}, maxConcurrentSamples),
	}
}

// rekeyByClaim restores the stable claim identity used by live observations.
func rekeyByClaim(
	usage map[string]model.PVCSample,
) map[string]model.PVCSample {
	result := make(map[string]model.PVCSample, len(usage))
	for _, sample := range usage {
		if sample.Namespace == "" || sample.Name == "" {
			continue
		}
		key := sample.Namespace + "/" + sample.Name
		if existing, ok := result[key]; ok && existing.Seen.After(sample.Seen) {
			continue
		}
		result[key] = sample
	}
	return result
}

func sortObservations(observations []*model.Observation) {
	sort.Slice(observations, func(i, j int) bool {
		left, right := observations[i].Subject, observations[j].Subject
		if left.Namespace != right.Namespace {
			return left.Namespace < right.Namespace
		}
		return left.Name < right.Name
	})
}

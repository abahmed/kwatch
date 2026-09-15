package probe

import (
	"net/http"
	"sync"
	"time"

	"k8s.io/client-go/kubernetes"
	corev1lister "k8s.io/client-go/listers/core/v1"

	"github.com/abahmed/kwatch/internal/config"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/monitor"
)

type Monitor struct {
	cfg          config.ActiveProbeMonitor
	incidentSink monitor.ObservationSink
	client       *http.Client
	resolver     HostResolver
	timeout      time.Duration
	kclient      kubernetes.Interface
	mu           sync.Mutex
	failures     map[string]int
	successes    map[string]int
	// failing marks the targets that have actually reported a failure. A
	// target that has always been healthy has nothing to recover from, so it
	// must not resolve on every tick.
	failing     map[string]bool
	graph       *kwcontext.ResourceGraph
	now         func() time.Time
	namespaces  []string
	watchAll    bool
	allowed     func(string) bool
	autoTargets map[string]autoProbeTarget
	// serviceLister reads the controller's Service informer cache. Auto
	// service probing used to LIST every Service in scope on every interval
	// (30s by default) while the controller already watched them.
	serviceLister corev1lister.ServiceLister
	configured    bool
	started       bool
}

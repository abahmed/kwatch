package probe

import (
	"context"
	"fmt"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
)

// SkipProbeAnnotation opts one Service out of automatic probing.
//
// Auto-probing opens a real TCP connection from kwatch's own pod, so a
// Service behind a NetworkPolicy that does not admit kwatch is reported as
// failing when it is perfectly healthy. The namespace-wide escape hatch is
// activeProbeMonitor.excludeNamespaces; this is the per-Service one, for the
// case where a single Service in an otherwise reachable namespace is closed
// off.
const SkipProbeAnnotation = "kwatch.io/skip-probe"

// skipAutoProbe reports whether a Service has opted out.
func skipAutoProbe(service *corev1.Service) bool {
	if service == nil {
		return true
	}
	switch strings.ToLower(service.Annotations[SkipProbeAnnotation]) {
	case "true", "yes", "1":
		return true
	}
	return false
}

type autoProbeTarget struct {
	owner     string
	graphNode string
}

type serviceProbe struct {
	port    corev1.ServicePort
	owner   string
	address string
}

func (m *Monitor) checkServices(ctx context.Context) {
	const pageSize int64 = 500
	const workers = 8
	namespaces, watchAll, allowed := m.namespaceSnapshot()
	if !watchAll && len(namespaces) == 0 {
		return
	}
	if watchAll {
		namespaces = []string{""}
	} else {
		namespaces = withoutExcluded(namespaces, m.excludedNamespaces())
	}
	jobs := make(chan serviceProbe)
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for probe := range jobs {
				m.checkServiceProbe(ctx, probe)
			}
		}()
	}
	current, complete := m.queueServiceProbes(
		ctx, namespaces, allowed, pageSize, jobs,
	)
	close(jobs)
	wg.Wait()
	if complete {
		m.reconcileAutoTargets(current)
	}
}

func (m *Monitor) namespaceSnapshot() ([]string, bool, func(string) bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.namespaces...), m.watchAll, m.allowed
}

// excludedNamespaces is the configured auto-probe exclusion set.
func (m *Monitor) excludedNamespaces() map[string]bool {
	if len(m.cfg.ExcludeNamespaces) == 0 {
		return nil
	}
	out := make(map[string]bool, len(m.cfg.ExcludeNamespaces))
	for _, ns := range m.cfg.ExcludeNamespaces {
		out[ns] = true
	}
	return out
}

// withoutExcluded drops the excluded namespaces from an explicit scope list.
// A cluster-wide scope cannot be filtered here -- it lists with an empty
// namespace -- so the per-Service check below carries that case.
func withoutExcluded(namespaces []string, excluded map[string]bool) []string {
	if len(excluded) == 0 {
		return namespaces
	}
	out := make([]string, 0, len(namespaces))
	for _, ns := range namespaces {
		if !excluded[ns] {
			out = append(out, ns)
		}
	}
	return out
}

// probeable reports whether a Service should be probed automatically.
func probeable(
	service *corev1.Service,
	allowed func(string) bool,
	excluded map[string]bool,
) bool {
	if excluded[service.Namespace] || skipAutoProbe(service) {
		return false
	}
	return allowed == nil || allowed(service.Namespace)
}

func (m *Monitor) queueServiceProbes(
	ctx context.Context,
	namespaces []string,
	allowed func(string) bool,
	pageSize int64,
	jobs chan<- serviceProbe,
) (map[string]autoProbeTarget, bool) {
	current := make(map[string]autoProbeTarget)
	for _, namespace := range namespaces {
		complete := m.queueNamespaceProbes(
			ctx, namespace, allowed, pageSize, jobs, current,
		)
		if !complete {
			return current, false
		}
	}
	return current, true
}

func (m *Monitor) queueNamespaceProbes(
	ctx context.Context,
	namespace string,
	allowed func(string) bool,
	pageSize int64,
	jobs chan<- serviceProbe,
	current map[string]autoProbeTarget,
) bool {
	excluded := m.excludedNamespaces()
	if cached := m.servicesFor(namespace); cached != nil {
		for _, service := range cached {
			if !probeable(service, allowed, excluded) {
				continue
			}
			for _, probe := range serviceProbes(service) {
				rememberAutoProbe(current, probe)
				select {
				case jobs <- probe:
				case <-ctx.Done():
					return false
				}
			}
		}
		return true
	}
	continueToken := ""
	for {
		services, err := m.kclient.CoreV1().Services(namespace).List(
			ctx,
			metav1.ListOptions{Limit: pageSize, Continue: continueToken},
		)
		if err != nil {
			return false
		}
		for i := range services.Items {
			service := &services.Items[i]
			if !probeable(service, allowed, excluded) {
				continue
			}
			for _, probe := range serviceProbes(service) {
				rememberAutoProbe(current, probe)
				select {
				case jobs <- probe:
				case <-ctx.Done():
					return false
				}
			}
		}
		if services.Continue == "" {
			return true
		}
		continueToken = services.Continue
	}
}

func rememberAutoProbe(
	current map[string]autoProbeTarget,
	probe serviceProbe,
) {
	current["auto-"+probe.owner] = autoProbeTarget{
		owner: probe.owner, graphNode: probe.owner,
	}
	if strings.HasPrefix(strings.ToLower(probe.port.Name), "http") {
		current["auto-http-"+probe.owner] = autoProbeTarget{
			owner: probe.owner + "/http", graphNode: probe.owner,
		}
	}
}

func serviceProbes(service *corev1.Service) []serviceProbe {
	host := service.Name + "." + service.Namespace + ".svc"
	probes := make([]serviceProbe, 0, len(service.Spec.Ports))
	for _, port := range service.Spec.Ports {
		if port.Port <= 0 ||
			(port.Protocol != "" && port.Protocol != corev1.ProtocolTCP) {
			continue
		}
		owner := fmt.Sprintf(
			"service/%s/%s/%d",
			service.Namespace,
			service.Name,
			port.Port,
		)
		probes = append(probes, serviceProbe{
			port: port, owner: owner,
			address: fmt.Sprintf("%s:%d", host, port.Port),
		})
	}
	return probes
}

func (m *Monitor) checkServiceProbe(ctx context.Context, probe serviceProbe) {
	m.linkTarget(probe.owner, probe.address)
	ok, detail := m.tcp(ctx, config.TCPProbeTarget{
		Name: probe.owner, Address: probe.address,
	})
	m.record(
		"auto-"+probe.owner, probe.owner,
		constant.ReasonActiveProbeFailure, ok, detail,
	)
	if !strings.HasPrefix(strings.ToLower(probe.port.Name), "http") {
		return
	}
	scheme := "http"
	if strings.HasPrefix(strings.ToLower(probe.port.Name), "https") {
		scheme = "https"
	}
	httpOK, detail, reason := m.http(ctx, config.HTTPProbeTarget{
		Name: probe.owner, URL: scheme + "://" + probe.address,
	})
	m.record(
		"auto-http-"+probe.owner, probe.owner+"/http",
		reason, httpOK, detail,
	)
}

func (m *Monitor) reconcileAutoTargets(current map[string]autoProbeTarget) {
	removed := m.replaceAutoTargets(current)
	removedGraph := make(map[string]struct{})
	for _, target := range removed {
		m.resolveRemovedTarget(target)
		if m.graph == nil || graphNodeStillUsed(current, target.graphNode) {
			continue
		}
		if _, done := removedGraph[target.graphNode]; !done {
			m.graph.RemoveNode("activeprobe", "", target.graphNode)
			removedGraph[target.graphNode] = struct{}{}
		}
	}
}

func graphNodeStillUsed(
	current map[string]autoProbeTarget,
	graphNode string,
) bool {
	for _, target := range current {
		if target.graphNode == graphNode {
			return true
		}
	}
	return false
}

func (m *Monitor) replaceAutoTargets(
	current map[string]autoProbeTarget,
) []autoProbeTarget {
	m.mu.Lock()
	defer m.mu.Unlock()
	removed := make([]autoProbeTarget, 0)
	for key, target := range m.autoTargets {
		if _, exists := current[key]; exists {
			continue
		}
		delete(m.failures, key)
		delete(m.successes, key)
		removed = append(removed, target)
	}
	m.autoTargets = current
	return removed
}

func (m *Monitor) resolveRemovedTarget(target autoProbeTarget) {
	for _, reason := range []string{
		constant.ReasonActiveProbeFailure,
		constant.ReasonActiveProbeLatency,
	} {
		m.correlator.Resolve(probeRef(target.owner), reason)
	}
}

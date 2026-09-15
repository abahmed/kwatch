package kubeletmetrics

import (
	"context"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	corev1lister "k8s.io/client-go/listers/core/v1"
)

func (m *Monitor) pruneSnapshots(nodes []corev1.Node) {
	active := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		active[node.Name] = struct{}{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for key := range m.previous {
		node, ok := snapshotNode(key)
		if ok {
			if _, exists := active[node]; !exists {
				delete(m.previous, key)
			}
		}
	}
	for node := range m.endpoint {
		if _, exists := active[node]; !exists {
			delete(m.endpoint, node)
		}
	}
}

func snapshotNode(key string) (string, bool) {
	parts := strings.SplitN(key, "/", 3)
	if len(parts) < 2 {
		return "", false
	}
	if parts[0] == "network" || parts[0] == "runtime" {
		return parts[1], true
	}
	if parts[0] == "cpu" && len(parts) == 3 {
		return parts[1], true
	}
	return "", false
}

func (m *Monitor) pods(ctx context.Context) map[string]*corev1.Pod {
	now := m.now()
	m.mu.Lock()
	if now.Sub(m.podCacheAt) < 15*time.Second && len(m.podCache) > 0 {
		cached := m.podCache
		m.mu.Unlock()
		return cached
	}
	lister := m.podLister
	m.mu.Unlock()
	if lister != nil {
		if result, ok := m.podsFromCache(lister); ok {
			m.mu.Lock()
			m.podCache, m.podCacheAt = result, now
			m.mu.Unlock()
			return result
		}
	}
	m.mu.Lock()
	namespaces := append([]string(nil), m.namespaces...)
	watchAll := m.watchAll
	allowed := m.allowed
	m.mu.Unlock()
	if !watchAll && len(namespaces) == 0 {
		return map[string]*corev1.Pod{}
	}
	result := make(map[string]*corev1.Pod)
	if watchAll {
		namespaces = []string{""}
	}
	for _, namespace := range namespaces {
		continueToken := ""
		for {
			pods, err := m.client.CoreV1().Pods(namespace).List(
				ctx, metav1.ListOptions{Limit: 500, Continue: continueToken},
			)
			if err != nil {
				return nil
			}
			for i := range pods.Items {
				pod := &pods.Items[i]
				if allowed != nil && !allowed(pod.Namespace) {
					continue
				}
				result[pod.Namespace+"/"+pod.Name] = pod
			}
			continueToken = pods.Continue
			if continueToken == "" {
				break
			}
		}
	}
	m.mu.Lock()
	m.podCache, m.podCacheAt = result, now
	m.mu.Unlock()
	return result
}

func (m *Monitor) nodes(ctx context.Context) ([]corev1.Node, error) {
	m.mu.Lock()
	lister := m.nodeLister
	m.mu.Unlock()
	if lister != nil {
		nodes, err := lister.List(labels.Everything())
		if err == nil {
			result := make([]corev1.Node, 0, len(nodes))
			for _, node := range nodes {
				result = append(result, *node)
			}
			return result, nil
		}
	}
	var result []corev1.Node
	continueToken := ""
	for {
		nodes, err := m.client.CoreV1().Nodes().List(
			ctx,
			metav1.ListOptions{Limit: 500, Continue: continueToken},
		)
		if err != nil {
			return nil, err
		}
		result = append(result, nodes.Items...)
		continueToken = nodes.Continue
		if continueToken == "" {
			return result, nil
		}
	}
}

// podsFromCache reads the pod informer cache, honouring namespace scope. A
// false result means the cache could not answer and the API fallback is used.
func (m *Monitor) podsFromCache(
	lister corev1lister.PodLister,
) (map[string]*corev1.Pod, bool) {
	m.mu.Lock()
	namespaces := append([]string(nil), m.namespaces...)
	watchAll := m.watchAll
	allowed := m.allowed
	m.mu.Unlock()
	if !watchAll && len(namespaces) == 0 {
		return map[string]*corev1.Pod{}, true
	}
	if watchAll {
		namespaces = []string{""}
	}
	result := make(map[string]*corev1.Pod)
	for _, namespace := range namespaces {
		var (
			pods []*corev1.Pod
			err  error
		)
		if namespace == "" {
			pods, err = lister.List(labels.Everything())
		} else {
			pods, err = lister.Pods(namespace).List(labels.Everything())
		}
		if err != nil {
			return nil, false
		}
		for _, pod := range pods {
			if allowed != nil && !allowed(pod.Namespace) {
				continue
			}
			result[pod.Namespace+"/"+pod.Name] = pod
		}
	}
	return result, true
}

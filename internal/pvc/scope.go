package pvc

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

func (p *PvcMonitor) setNamespaceScopeLocked(
	allowed, forbidden []string, all ...bool,
) {
	p.allowedNamespaces = make(map[string]struct{}, len(allowed))
	for _, namespace := range allowed {
		p.allowedNamespaces[namespace] = struct{}{}
	}
	p.forbiddenNamespaces = make(map[string]struct{}, len(forbidden))
	for _, namespace := range forbidden {
		p.forbiddenNamespaces[namespace] = struct{}{}
	}
	p.watchAll = len(allowed) == 0
	if len(all) > 0 {
		p.watchAll = all[0]
	}
	p.pvByPVC = nil
	p.pvByPVCAt = time.Time{}
}

func (p *PvcMonitor) listPVCs(
	ctx context.Context,
) ([]corev1.PersistentVolumeClaim, error) {
	p.mu.RLock()
	namespaces := make([]string, 0, len(p.allowedNamespaces))
	for namespace := range p.allowedNamespaces {
		namespaces = append(namespaces, namespace)
	}
	watchAll := p.watchAll
	p.mu.RUnlock()
	if watchAll {
		namespaces = []string{""}
	}
	var result []corev1.PersistentVolumeClaim
	for _, namespace := range namespaces {
		continueToken := ""
		for {
			list, err := p.client.CoreV1().PersistentVolumeClaims(namespace).List(
				ctx, metav1.ListOptions{Limit: 500, Continue: continueToken},
			)
			if err != nil {
				return nil, err
			}
			result = append(result, list.Items...)
			continueToken = list.Continue
			if continueToken == "" {
				break
			}
		}
	}
	return result, nil
}

func (p *PvcMonitor) namespaceAllowed(namespace string) bool {
	p.mu.RLock()
	filter := p.namespaceFilter
	allowed := p.namespaceAllowedLocked(namespace)
	p.mu.RUnlock()
	if filter != nil {
		return filter(namespace)
	}
	return allowed
}

func (p *PvcMonitor) namespaceAllowedLocked(namespace string) bool {
	if _, forbidden := p.forbiddenNamespaces[namespace]; forbidden {
		return false
	}
	if len(p.allowedNamespaces) > 0 {
		_, allowed := p.allowedNamespaces[namespace]
		return allowed
	}
	return true
}

const pvByPVCTTL = 60 * time.Second

// pvcMap returns the cached PVC-to-PV map, refreshing it at most once per TTL.
func (p *PvcMonitor) pvcMap(ctx context.Context) map[string]string {
	p.mu.RLock()
	if p.pvByPVC != nil && p.now().Sub(p.pvByPVCAt) < pvByPVCTTL {
		m := p.pvByPVC
		p.mu.RUnlock()
		return clonePVMap(m)
	}
	p.mu.RUnlock()

	m := make(map[string]string)
	if p.client == nil {
		return m
	}
	if pvcs, err := p.listPVCs(ctx); err == nil {
		for i := range pvcs {
			claim := &pvcs[i]
			if !p.namespaceAllowed(claim.Namespace) {
				continue
			}
			m[claim.Namespace+"/"+claim.Name] = claim.Spec.VolumeName
		}
		p.mu.Lock()
		p.pvByPVC, p.pvByPVCAt = m, p.now()
		p.mu.Unlock()
	} else {
		klog.ErrorS(err, "pvc monitor: list PVCs")
		p.mu.RLock()
		m = clonePVMap(p.pvByPVC)
		p.mu.RUnlock()
	}
	return m
}

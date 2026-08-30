package handler

import (
	"fmt"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
)

const (
	nodeLeaseNamespace       = "kube-node-lease"
	defaultNodeLeaseStaleSec = 90
)

func (h *handler) ProcessLease(key string, deleted bool) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid lease key %q: %w", key, err)
	}
	if namespace != nodeLeaseNamespace {
		return nil
	}
	if deleted {
		h.resolveNodeLease(name)
		return nil
	}
	lease, err := h.listers.Lease.Leases(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.resolveNodeLease(name)
			return nil
		}
		return fmt.Errorf("failed to get lease %s from cache: %w", key, err)
	}
	if sig := DetectNodeLeaseIssue(lease, h.now(), h.config.ClusterResourceMonitor.NodeLeaseStaleSeconds); sig != nil {
		h.signalEvent(sig)
	} else {
		h.resolveNodeLease(name)
	}
	return nil
}

func DetectNodeLeaseIssue(lease *coordinationv1.Lease, now time.Time, staleSeconds int) *event.Signal {
	if lease == nil || lease.Namespace != nodeLeaseNamespace {
		return nil
	}
	if staleSeconds <= 0 {
		staleSeconds = defaultNodeLeaseStaleSec
	}
	if lease.Spec.RenewTime == nil || now.Sub(lease.Spec.RenewTime.Time) > time.Duration(staleSeconds)*time.Second {
		age := "never"
		if lease.Spec.RenewTime != nil {
			age = now.Sub(lease.Spec.RenewTime.Time).Round(time.Second).String()
		}
		return &event.Signal{
			Resource: "node", NodeName: lease.Name, PodName: lease.Name,
			Owner: lease.Name, Reason: constant.ReasonNodeLeaseStale,
			Labels: lease.Labels,
			Hint:   fmt.Sprintf("node lease %s/%s has not renewed for %s; kubelet may be unavailable", lease.Namespace, lease.Name, age),
		}
	}
	return nil
}

func (h *handler) resolveNodeLease(nodeName string) {
	h.correlator.MarkResolved(correlation.BuildKey("", nodeName, constant.ReasonNodeLeaseStale, ""))
}

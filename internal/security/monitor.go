package security

import (
	"context"
	"sync"
	"time"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

type Permission struct {
	Group    string `json:"group"`
	Resource string `json:"resource"`
	Verb     string `json:"verb"`
}

type Status struct {
	LastCheck  time.Time    `json:"lastCheck"`
	Available  bool         `json:"available"`
	RBACDenied bool         `json:"rbacDenied"`
	Missing    []Permission `json:"missing,omitempty"`
	Checks     int          `json:"checks"`
}

type Monitor struct {
	client kubernetes.Interface
	mu     sync.RWMutex
	status Status
}

var requiredClusterPermissions = clusterPermissions()

func clusterPermissions() []Permission {
	resources := []Permission{
		{Resource: "nodes"}, {Resource: "namespaces"}, {Resource: "persistentvolumes"},
		{Resource: "storageclasses", Group: "storage.k8s.io"},
		{Resource: "volumeattachments", Group: "storage.k8s.io"},
		{Resource: "apiservices", Group: "apiregistration.k8s.io"},
		{Resource: "customresourcedefinitions", Group: "apiextensions.k8s.io"},
		{Resource: "mutatingwebhookconfigurations", Group: "admissionregistration.k8s.io"},
		{Resource: "validatingwebhookconfigurations", Group: "admissionregistration.k8s.io"},
	}
	permissions := make([]Permission, 0, len(resources)*3)
	for _, resource := range resources {
		for _, verb := range []string{"get", "list", "watch"} {
			resource.Verb = verb
			permissions = append(permissions, resource)
		}
	}
	return permissions
}

func New(client kubernetes.Interface) *Monitor {
	return &Monitor{client: client}
}

func (m *Monitor) Start(ctx context.Context) {
	if m.client == nil {
		return
	}
	interval := 5 * time.Minute
	m.check(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.check(ctx)
		}
	}
}

func (m *Monitor) Snapshot() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	status := m.status
	status.Missing = append([]Permission(nil), m.status.Missing...)
	return status
}

func (m *Monitor) SecurityStatus() interface{} {
	return m.Snapshot()
}

func (m *Monitor) check(ctx context.Context) {
	requestCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	status := Status{Available: true, Checks: len(requiredClusterPermissions), LastCheck: time.Now()}
	for _, permission := range requiredClusterPermissions {
		allowed, err := m.allowed(requestCtx, permission)
		if err != nil {
			status.Available = false
			if requestCtx.Err() != nil {
				break
			}
			klog.V(2).InfoS("security RBAC check unavailable", "resource", permission.Resource, "error", err)
			continue
		}
		if !allowed {
			status.RBACDenied = true
			status.Missing = append(status.Missing, permission)
		}
	}
	m.mu.Lock()
	m.status = status
	m.mu.Unlock()
}

func (m *Monitor) allowed(ctx context.Context, permission Permission) (bool, error) {
	group := permission.Group
	request := &authorizationv1.SelfSubjectAccessReview{
		Spec: authorizationv1.SelfSubjectAccessReviewSpec{ResourceAttributes: &authorizationv1.ResourceAttributes{
			Group: group, Resource: permission.Resource, Verb: permission.Verb,
		}},
	}
	result, err := m.client.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, request, metav1.CreateOptions{})
	if err != nil {
		return false, err
	}
	return result.Status.Allowed, nil
}

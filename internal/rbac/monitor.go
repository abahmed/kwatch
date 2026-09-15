package rbac

import (
	"context"
	"sync"
	"time"

	authorizationv1 "k8s.io/api/authorization/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
)

type Permission struct {
	Namespace      string `json:"namespace,omitempty"`
	Group          string `json:"group"`
	Resource       string `json:"resource"`
	Name           string `json:"name,omitempty"`
	Verb           string `json:"verb"`
	NonResourceURL string `json:"nonResourceURL,omitempty"`
}

type Status struct {
	State      string       `json:"state"`
	LastCheck  time.Time    `json:"lastCheck"`
	Available  bool         `json:"available"`
	RBACDenied bool         `json:"rbacDenied"`
	Missing    []Permission `json:"missing,omitempty"`
	Checks     int          `json:"checks"`
	Scope      string       `json:"scope"`
}

type Monitor struct {
	client                kubernetes.Interface
	mu                    sync.RWMutex
	status                Status
	namespaces            []string
	allNamespaces         bool
	clusterPermissions    []Permission
	namespacedPermissions []Permission
	infrastructure        []Permission
	now                   func() time.Time
	configured            bool
	started               bool
	// cycle counts sweeps, for the namespace round-robin.
	cycle int
	// fullSweep asks the next sweep to check every namespace. Set on the
	// first run and after any denial.
	fullSweep bool
	// lastMissing remembers each namespace's last result, so a sweep that
	// samples one namespace still reports the whole picture.
	lastMissing map[string][]Permission
}

func namespacedPermissions() []Permission {
	resources := []Permission{
		{Resource: "pods"}, {Resource: "events"}, {Resource: "services"},
		{Resource: "endpointslices", Group: "discovery.k8s.io"},
		{Resource: "deployments", Group: "apps"},
		{Resource: "replicasets", Group: "apps"},
		{Resource: "statefulsets", Group: "apps"},
		{Resource: "daemonsets", Group: "apps"},
		{Resource: "jobs", Group: "batch"}, {Resource: "cronjobs", Group: "batch"},
		{Resource: "horizontalpodautoscalers", Group: "autoscaling"},
		{Resource: "poddisruptionbudgets", Group: "policy"},
		{Resource: "networkpolicies", Group: "networking.k8s.io"},
		{Resource: "resourcequotas"}, {Resource: "limitranges"},
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

func clusterPermissions() []Permission {
	resources := []Permission{
		{Resource: "nodes"}, {Resource: "namespaces"},
		{Resource: "persistentvolumes"},
		{Resource: "storageclasses", Group: "storage.k8s.io"},
		{Resource: "volumeattachments", Group: "storage.k8s.io"},
		{Resource: "apiservices", Group: "apiregistration.k8s.io"},
		{Resource: "customresourcedefinitions", Group: "apiextensions.k8s.io"},
		{Resource: "mutatingwebhookconfigurations",
			Group: "admissionregistration.k8s.io"},
		{Resource: "validatingwebhookconfigurations",
			Group: "admissionregistration.k8s.io"},
	}
	permissions := make([]Permission, 0, len(resources)*3)
	for _, resource := range resources {
		for _, verb := range []string{"get", "list", "watch"} {
			resource.Verb = verb
			permissions = append(permissions, resource)
		}
	}
	permissions = append(permissions, Permission{
		Resource: "selfsubjectaccessreviews",
		Group:    "authorization.k8s.io",
		Verb:     "create",
	})
	return permissions
}

// NewWithRuntimeConfig builds the permission auditor from the immutable
// runtime snapshot. The application uses this constructor so RBAC checks
// cannot observe a partially normalized YAML configuration.
func NewWithRuntimeConfig(
	client kubernetes.Interface,
	runtime config.RuntimeConfig,
	timeSource clock.Clock,
) *Monitor {
	if timeSource == nil {
		timeSource = clock.RealClock{}
	}
	return newConfiguredMonitor(client, runtime, timeSource.Now, false)
}

func newConfiguredMonitor(
	client kubernetes.Interface,
	runtime config.RuntimeConfig,
	now func() time.Time,
	fullPermissions bool,
) *Monitor {
	var cluster, namespaced []Permission
	if fullPermissions {
		cluster, namespaced = clusterPermissions(), namespacedPermissions()
	} else {
		cluster, namespaced = permissionsForRuntime(runtime)
	}
	return &Monitor{
		client:                client,
		clusterPermissions:    cluster,
		namespacedPermissions: namespaced,
		infrastructure:        infrastructurePermissionsForRuntime(runtime),
		now:                   now,
	}
}

func (m *Monitor) nowTime() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Time{}
}

func (m *Monitor) check(ctx context.Context) {
	requestCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	m.mu.Lock()
	namespaces := append([]string(nil), m.namespaces...)
	allNamespaces := m.allNamespaces
	infrastructure := append([]Permission(nil), m.infrastructure...)
	full := m.fullSweep || m.lastMissing == nil
	if m.lastMissing == nil {
		m.lastMissing = map[string][]Permission{}
	}
	m.cycle++
	cycle := m.cycle
	m.mu.Unlock()
	if allNamespaces {
		namespaces = []string{""}
	}
	status := Status{
		Available: true, Checks: len(m.clusterPermissions),
		LastCheck: m.nowTime(), Scope: "cluster",
	}
	for _, permission := range m.clusterPermissions {
		allowed, err := m.allowed(requestCtx, permission)
		if err != nil {
			status.Available = false
			if apierrors.IsForbidden(err) {
				status.RBACDenied = true
			}
			if requestCtx.Err() != nil {
				break
			}
			klog.V(2).InfoS(
				"security RBAC check unavailable",
				"resource", permission.Resource, "error", err,
			)
			continue
		}
		if !allowed {
			status.RBACDenied = true
			status.Missing = append(status.Missing, permission)
		}
	}
	for _, permission := range infrastructure {
		allowed, err := m.allowed(requestCtx, permission)
		status.Checks++
		if err != nil {
			status.Available = false
			if apierrors.IsForbidden(err) {
				status.RBACDenied = true
			}
			continue
		}
		if !allowed {
			status.RBACDenied = true
			status.Missing = append(status.Missing, permission)
		}
	}
	m.checkNamespaces(requestCtx, namespaces, sampled(namespaces, full, cycle),
		&status)

	m.mu.Lock()
	m.status = status
	// A denial anywhere means the next sweep checks everything: the sampled
	// picture is the one that must not be trusted when something is wrong.
	m.fullSweep = status.RBACDenied || !status.Available
	m.mu.Unlock()
}

// sampled picks the namespaces this sweep actually queries.
//
// Checking every namespace meant roughly forty-five SelfSubjectAccessReviews
// per namespace per sweep, to re-confirm grants that change about never. One
// namespace per sweep, round-robin, finds a revoked grant within a full
// rotation; the remembered result stands in for the rest, and a denial
// promotes the next sweep back to a full one.
func sampled(namespaces []string, full bool, cycle int) map[string]bool {
	out := make(map[string]bool, len(namespaces))
	if full || len(namespaces) <= 1 {
		for _, ns := range namespaces {
			out[ns] = true
		}
		return out
	}
	out[namespaces[cycle%len(namespaces)]] = true
	return out
}

// checkNamespaces runs the namespaced permission checks for the sampled
// namespaces and folds every namespace's latest known result into status.
func (m *Monitor) checkNamespaces(
	ctx context.Context,
	namespaces []string,
	check map[string]bool,
	status *Status,
) {
	for _, namespace := range namespaces {
		if namespace != "" {
			status.Scope = "cluster+namespace"
		}
		if !check[namespace] {
			m.mu.RLock()
			status.Missing = append(status.Missing, m.lastMissing[namespace]...)
			m.mu.RUnlock()
			continue
		}
		var missing []Permission
		for _, permission := range m.namespacedPermissions {
			permission.Namespace = namespace
			allowed, err := m.allowed(ctx, permission)
			status.Checks++
			if err != nil {
				status.Available = false
				if apierrors.IsForbidden(err) {
					status.RBACDenied = true
				}
				continue
			}
			if !allowed {
				missing = append(missing, permission)
			}
		}
		status.Missing = append(status.Missing, missing...)
		m.mu.Lock()
		m.lastMissing[namespace] = missing
		m.mu.Unlock()
	}
	if len(status.Missing) > 0 {
		status.RBACDenied = true
	}
}

func (m *Monitor) allowed(
	ctx context.Context, permission Permission,
) (bool, error) {
	request := &authorizationv1.SelfSubjectAccessReview{
		Spec: authorizationv1.SelfSubjectAccessReviewSpec{},
	}
	if permission.NonResourceURL != "" {
		request.Spec.NonResourceAttributes = &authorizationv1.NonResourceAttributes{
			Path: permission.NonResourceURL,
			Verb: permission.Verb,
		}
	} else {
		request.Spec.ResourceAttributes = &authorizationv1.ResourceAttributes{
			Namespace: permission.Namespace,
			Group:     permission.Group,
			Resource:  permission.Resource,
			Name:      permission.Name,
			Verb:      permission.Verb,
		}
	}
	result, err := m.client.AuthorizationV1().SelfSubjectAccessReviews().Create(
		ctx, request, metav1.CreateOptions{},
	)
	if err != nil {
		return false, err
	}
	return result.Status.Allowed, nil
}

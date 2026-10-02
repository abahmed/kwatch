package rbac

import (
	"context"
	"strings"
	"time"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// checkInterval is deliberately slow: grants change rarely and every check
// is one SelfSubjectAccessReview per permission.
const (
	checkInterval = 15 * time.Minute
	checkTimeout  = 15 * time.Second
)

// Status is the result of one permission sweep.
type Status struct {
	LastCheck time.Time
	// Unavailable is true when the review API itself failed.
	Unavailable bool
	// Missing lists denied permissions.
	Missing []kube.Access
}

// RequiredMissing reports whether a required permission is denied.
func (s Status) RequiredMissing() bool {
	for _, access := range s.Missing {
		if access.Required {
			return true
		}
	}
	return false
}

// Monitor periodically verifies the permissions kwatch uses and reports
// each sweep. It never changes behavior: sources degrade on their own.
type Monitor struct {
	client kubernetes.Interface
	checks []kube.Access
	clock  clock.Clock
	report func(Status)
}

// NewMonitor builds a monitor for checks. report receives every sweep.
func NewMonitor(
	client kubernetes.Interface, checks []kube.Access,
	clk clock.Clock, report func(Status),
) *Monitor {
	return &Monitor{
		client: client, checks: checks, clock: clock.Require(clk),
		report: report,
	}
}

// Checks is every permission kwatch uses: the sources' access, the Lease
// named lease that kwatch holds in its own namespace, and the KwatchConfig
// resources when the CRD watcher is enabled. Get and update on the Lease
// are checked by name because the Role grants them only for that name;
// create cannot be limited by name in Kubernetes RBAC.
func Checks(namespace, lease string, crdEnabled bool) []kube.Access {
	checks := append(kube.SourceAccess(),
		// The cluster ID is the kube-system Namespace UID.
		kube.Access{Resource: kube.Resource{Name: "namespaces"},
			Verb: "get", Required: true})
	checks = append(checks, kube.InstallAccess(namespace)...)
	leases := kube.Resource{Group: "coordination.k8s.io", Name: "leases"}
	for _, verb := range []string{"get", "create", "update"} {
		name := lease
		if verb == "create" {
			name = ""
		}
		checks = append(checks, kube.Access{Resource: leases, Verb: verb,
			Namespace: namespace, Name: name, Required: true})
	}
	if crdEnabled {
		config := kube.Resource{Group: "kwatch.abahmed.dev",
			Name: "kwatchconfigs"}
		for _, verb := range []string{"list", "watch"} {
			checks = append(checks, kube.Access{Resource: config,
				Verb: verb, Namespace: namespace})
		}
	}
	return checks
}

// Start sweeps at once and then every checkInterval until ctx ends.
func (m *Monitor) Start(ctx context.Context) error {
	if m.client == nil {
		return nil
	}
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	for {
		m.report(m.Sweep(ctx))
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// Sweep checks every permission once.
func (m *Monitor) Sweep(ctx context.Context) Status {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	status := Status{LastCheck: m.clock.Now()}
	for _, access := range m.checks {
		allowed, err := m.allowed(ctx, access)
		if err != nil {
			klog.V(2).InfoS("permission review unavailable",
				"component", "rbac", "error", err)
			status.Unavailable = true
			return status
		}
		if !allowed {
			status.Missing = append(status.Missing, access)
		}
	}
	return status
}

func (m *Monitor) allowed(
	ctx context.Context, access kube.Access,
) (bool, error) {
	spec := authorizationv1.SelfSubjectAccessReviewSpec{}
	if access.NonResourceURL != "" {
		spec.NonResourceAttributes = &authorizationv1.NonResourceAttributes{
			Path: access.NonResourceURL, Verb: access.Verb,
		}
	} else {
		resource, subresource, _ := strings.Cut(access.Resource.Name, "/")
		spec.ResourceAttributes = &authorizationv1.ResourceAttributes{
			Namespace: access.Namespace, Group: access.Resource.Group,
			Resource: resource, Subresource: subresource,
			Name: access.Name, Verb: access.Verb,
		}
	}
	result, err := m.client.AuthorizationV1().SelfSubjectAccessReviews().
		Create(ctx, &authorizationv1.SelfSubjectAccessReview{Spec: spec},
			metav1.CreateOptions{})
	if err != nil {
		return false, err
	}
	return result.Status.Allowed, nil
}

package rbac

import (
	"context"
	"testing"
	"time"

	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/abahmed/kwatch/internal/clock"
)

func TestRBACStartGuardsAndPermissionChecks(t *testing.T) {
	monitor := &Monitor{}
	if err := monitor.Start(context.Background()); err != nil {
		t.Fatalf("unconfigured Start() error = %v", err)
	}
	if monitor.Snapshot().State != "unavailable" {
		t.Fatal("unconfigured monitor was not unavailable")
	}
	monitor = &Monitor{configured: true}
	if err := monitor.Start(context.Background()); err != nil {
		t.Fatalf("nil-client Start() error = %v", err)
	}
	client := fake.NewSimpleClientset()
	client.PrependReactor("create", "selfsubjectaccessreviews",
		func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, &authorizationv1.SelfSubjectAccessReview{
				Status: authorizationv1.SubjectAccessReviewStatus{Allowed: true},
			}, nil
		})
	monitor = &Monitor{
		client:                client,
		clusterPermissions:    []Permission{{Resource: "pods", Verb: "list"}},
		namespacedPermissions: []Permission{{Resource: "services", Verb: "get"}},
		infrastructure:        []Permission{{Resource: "configmaps", Verb: "get"}},
		now:                   clock.RealClock{}.Now,
	}
	if err := monitor.ConfigureSources(Sources{
		Namespaces: []string{"apps"}, InfrastructureNamespace: "kwatch",
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("configured Start() error = %v", err)
	}
	if monitor.status.Checks == 0 {
		t.Fatal("configured monitor did not perform checks")
	}
	if _, err := monitor.allowed(context.Background(), Permission{
		NonResourceURL: "/healthz", Verb: "get",
	}); err != nil {
		t.Fatalf("non-resource permission check = %v", err)
	}
	if _, err := monitor.allowed(context.Background(), Permission{
		Group: "apps", Resource: "deployments", Verb: "list",
	}); err != nil {
		t.Fatalf("resource permission check = %v", err)
	}
	if len(client.Actions()) == 0 {
		t.Fatal("permission checks did not call the client")
	}
}

func TestRBACCheckRecordsDeniedPermissions(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("create", "selfsubjectaccessreviews",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, &authorizationv1.SelfSubjectAccessReview{
				Status: authorizationv1.SubjectAccessReviewStatus{
					Allowed: false,
				},
			}, nil
		})
	monitor := &Monitor{
		client: client, clusterPermissions: []Permission{{
			Resource: "pods", Verb: "list",
		}}, now: time.Now,
		lastMissing: make(map[string][]Permission),
	}
	monitor.check(context.Background())
	if !monitor.Snapshot().RBACDenied || len(monitor.Snapshot().Missing) == 0 {
		t.Fatal("denied permission was not recorded")
	}
}

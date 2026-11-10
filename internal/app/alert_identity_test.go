package app

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/kubeclient"
)

func identityDeps(name string, objects ...*corev1.Namespace) *serverDeps {
	client := fake.NewSimpleClientset()
	for _, ns := range objects {
		_ = client.Tracker().Add(ns)
	}
	return &serverDeps{
		runtime: config.RuntimeConfigFor(&config.Config{
			App: config.App{ClusterName: name},
		}),
		clients: kubeclient.ClientSet{Kubernetes: client},
	}
}

// Only a cluster without a name gets the fallback identity: a named
// cluster's alert keys must not change.
func TestAlertIdentityIsUsedOnlyWithoutAClusterName(t *testing.T) {
	kubeSystem := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name: "kube-system", UID: "uid-1",
	}}
	ctx := context.Background()

	if got := alertIdentity(ctx, identityDeps("", kubeSystem)); got != "uid-1" {
		t.Fatalf("identity = %q, want the kube-system UID", got)
	}
	if got := alertIdentity(ctx, identityDeps("east", kubeSystem)); got != "" {
		t.Fatalf("a named cluster got identity %q", got)
	}
	if got := alertIdentity(ctx, identityDeps("")); got != "" {
		t.Fatalf("a failed lookup gave identity %q", got)
	}
}

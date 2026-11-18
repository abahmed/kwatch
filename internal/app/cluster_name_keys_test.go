package app

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/kubeclient"
	"github.com/abahmed/kwatch/internal/notification"
)

// keyCluster is a cluster as kwatch sees it at one start: the client it
// reads, the configured name, and whether the node list fails.
type keyCluster struct {
	configured string
	nodesFail  bool
}

func (c keyCluster) client() *fake.Clientset {
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system", UID: "uid-1"}},
		nodeWith("n1", map[string]string{
			"alpha.eksctl.io/cluster-name": "alpha-1"}))
	if c.nodesFail {
		client.PrependReactor("list", "nodes", func(
			clienttesting.Action,
		) (bool, runtime.Object, error) {
			return true, nil, context.DeadlineExceeded
		})
	}
	return client
}

// start returns what a start of kwatch shows and the key a provider
// gives an open alert whose incident was keyed before the upgrade.
func (c keyCluster) start(t *testing.T) (shown, key, identity string) {
	t.Helper()
	ctx := context.Background()
	client := c.client()
	cfg := &config.Config{App: config.App{ClusterName: c.configured}}
	rt, name, _ := clusterRuntime(ctx, cfg, client)
	deps := &serverDeps{runtime: rt, displayName: name,
		clients: kubeclient.ClientSet{Kubernetes: client}}
	open := notification.Message{Key: "incident-1",
		DedupKey: "incident-1-abc"}
	return deps.clusterName(),
		open.AlertKey(rt.Application().ClusterName),
		alertIdentity(ctx, deps)
}

// An upgrade that starts detecting the cluster name shows the name but
// keeps the key of every open alert and the identity in new keys; a
// later start whose detection fails gives the same keys again.
func TestDetectedClusterNameKeepsAlertKeys(t *testing.T) {
	const before = "kwatch-incident-1-abc"
	shown, key, identity := keyCluster{}.start(t)
	if shown != "alpha-1" {
		t.Fatalf("shown name = %q, want the detected one", shown)
	}
	if key != before || identity != "uid-1" {
		t.Fatalf("key %q identity %q, want %q and the UID",
			key, identity, before)
	}
	shown, key, identity = keyCluster{nodesFail: true}.start(t)
	if shown != "" || key != before || identity != "uid-1" {
		t.Fatalf("failed detection: shown %q key %q identity %q",
			shown, key, identity)
	}
}

// A configured name is shown and is part of every key, as before.
func TestConfiguredClusterNameStaysInAlertKeys(t *testing.T) {
	shown, key, identity := keyCluster{configured: "east"}.start(t)
	if shown != "east" || key != "kwatch-east-incident-1-abc" ||
		identity != "" {
		t.Fatalf("shown %q key %q identity %q", shown, key, identity)
	}
}

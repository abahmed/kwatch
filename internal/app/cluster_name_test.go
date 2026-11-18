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
)

func labelled(kv ...string) nodeMeta {
	n := nodeMeta{Labels: map[string]string{},
		Annotations: map[string]string{}}
	for i := 0; i+1 < len(kv); i += 2 {
		n.Labels[kv[i]] = kv[i+1]
	}
	return n
}

func annotated(kv ...string) nodeMeta {
	n := labelled()
	for i := 0; i+1 < len(kv); i += 2 {
		n.Annotations[kv[i]] = kv[i+1]
	}
	return n
}

func TestClusterNameFromNodes(t *testing.T) {
	const eks = "alpha.eksctl.io/cluster-name"
	const capi = "cluster.x-k8s.io/cluster-name"
	cases := []struct {
		name  string
		nodes []nodeMeta
		want  string
		from  string
	}{
		{"no nodes", nil, "", ""},
		{"no source", []nodeMeta{labelled("a", "b")}, "", ""},
		{"eksctl label", []nodeMeta{labelled(eks, "alpha-1"),
			labelled(eks, "alpha-1"), labelled()},
			"alpha-1", "from the node label " + eks},
		{"capi annotation", []nodeMeta{annotated(capi, "beta-2")},
			"beta-2", "from the node annotation " + capi},
		{"capi label", []nodeMeta{labelled(capi, "beta-2")},
			"beta-2", "from the node label " + capi},
		{"owned tag label", []nodeMeta{
			labelled("kubernetes.io/cluster/gamma-3", "owned")},
			"gamma-3", "from the node label kubernetes.io/cluster/<name>"},
		{"tag with another value is not a tag", []nodeMeta{
			labelled("kubernetes.io/cluster/gamma-3", "x")}, "", ""},
		{"nodes disagree", []nodeMeta{labelled(eks, "one"),
			labelled(eks, "two")}, "", ""},
		{"sources disagree", []nodeMeta{labelled(eks, "one"),
			annotated(capi, "two")}, "", ""},
		{"sources agree, first named", []nodeMeta{labelled(eks, "one"),
			annotated(capi, "one")}, "one", "from the node label " + eks},
		{"two tags on one node", []nodeMeta{labelled(
			"kubernetes.io/cluster/a1", "owned",
			"kubernetes.io/cluster/b2", "shared")}, "", ""},
		{"not a name", []nodeMeta{labelled(eks, "has space")}, "", ""},
		{"empty value", []nodeMeta{labelled(eks, " ")}, "", ""},
		{"generic default", []nodeMeta{labelled(eks, "kubernetes")},
			"", ""},
		{"disagreement beats a clear one", []nodeMeta{
			labelled(eks, "one"), labelled(eks, "bad name")}, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name, from := clusterNameFromNodes(tc.nodes)
			if name != tc.want || from != tc.from {
				t.Fatalf("got (%q, %q), want (%q, %q)",
					name, from, tc.want, tc.from)
			}
		})
	}
}

func nodeWith(name string, labels map[string]string) *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name: name, Labels: labels}}
}

func kubeadmMap(conf string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "kubeadm-config",
			Namespace: "kube-system"},
		Data: map[string]string{"ClusterConfiguration": conf},
	}
}

func TestWithClusterNameDetection(t *testing.T) {
	eks := map[string]string{"alpha.eksctl.io/cluster-name": "alpha-1"}
	cases := []struct {
		name     string
		set      string
		objects  []runtime.Object
		want     string
		wantFrom string
	}{
		{"configured wins", "chosen",
			[]runtime.Object{nodeWith("n1", eks)}, "chosen",
			nameConfigured},
		{"detected from nodes", "",
			[]runtime.Object{nodeWith("n1", eks)}, "alpha-1",
			"from the node label alpha.eksctl.io/cluster-name"},
		{"nodes disagree", "", []runtime.Object{nodeWith("n1", eks),
			nodeWith("n2", map[string]string{
				"alpha.eksctl.io/cluster-name": "other"})}, "", ""},
		{"nothing to read", "", nil, "", ""},
		{"kubeadm name", "", []runtime.Object{nodeWith("n1", nil),
			kubeadmMap("clusterName: delta-4\n")}, "delta-4",
			"from the kubeadm-config ConfigMap in kube-system"},
		{"kubeadm default is no name", "", []runtime.Object{
			kubeadmMap("clusterName: kubernetes\n")}, "", ""},
		{"nodes beat kubeadm", "", []runtime.Object{
			nodeWith("n1", eks), kubeadmMap("clusterName: delta-4\n")},
			"alpha-1",
			"from the node label alpha.eksctl.io/cluster-name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{App: config.App{ClusterName: tc.set}}
			got, from := clusterDisplayName(context.Background(), cfg,
				fake.NewSimpleClientset(tc.objects...))
			if got != tc.want || from != tc.wantFrom {
				t.Fatalf("got (%q, %q), want (%q, %q)",
					got, from, tc.want, tc.wantFrom)
			}
			if cfg.App.ClusterName != tc.set {
				t.Fatal("the loaded configuration was changed")
			}
		})
	}
}

// A configured name reads nothing from the cluster.
func TestWithClusterNameConfiguredReadsNothing(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("*", "*", func(
		clienttesting.Action,
	) (bool, runtime.Object, error) {
		t.Error("a configured name caused an API call")
		return false, nil, nil
	})
	cfg := &config.Config{App: config.App{ClusterName: " east "}}
	if _, from := clusterDisplayName(
		context.Background(), cfg, client); from != nameConfigured {
		t.Fatalf("source = %q", from)
	}
}

// A refused or failing read is a missing name, not a failure.
func TestWithClusterNameSurvivesReadFailure(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("list", "*", func(
		clienttesting.Action,
	) (bool, runtime.Object, error) {
		return true, nil, context.DeadlineExceeded
	})
	got, from := clusterDisplayName(context.Background(),
		&config.Config{}, client)
	if got != "" || from != "" {
		t.Fatalf("got (%q, %q)", got, from)
	}
	if g, f := clusterDisplayName(context.Background(),
		&config.Config{}, nil); g != "" || f != "" {
		t.Fatal("nil client gave a name")
	}
}

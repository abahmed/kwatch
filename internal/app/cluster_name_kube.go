package app

import (
	"context"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
)

// clusterNameTimeout bounds the lookups made at start; they read two
// small lists the application's own role already allows.
const clusterNameTimeout = 10 * time.Second

// nodeSampleLimit is how many nodes the lookup reads. Nodes of one
// cluster carry the same name, so a page is enough to find it.
const nodeSampleLimit = 500

// noNameNotice is what the log and the kwatch page say once when the
// cluster has no name.
const noNameNotice = "This cluster has no name. Set " + noNameHint +
	" so alerts and the dashboard say which cluster they are about."

// clusterDisplayName is the name the cluster is shown by (messages,
// dashboard, self-alerts) and says where it came from. A configured
// name always wins and nothing is read. Without one, the name is read
// from the nodes and then the kubeadm ConfigMap (see
// clusterNameFromNodes); with none found kwatch says so once and both
// are "". A detected name is for display only: alert keys and tracker
// markers keep the configured name, which stays in cfg, so they do not
// change when detection starts naming the cluster or later fails.
func clusterDisplayName(
	ctx context.Context, cfg *config.Config, client kubernetes.Interface,
) (name, from string) {
	if cfg == nil {
		return "", ""
	}
	if strings.TrimSpace(cfg.App.ClusterName) != "" {
		return cfg.App.ClusterName, nameConfigured
	}
	name, from = detectClusterName(ctx, client)
	if name == "" {
		klog.Warning(noNameNotice)
		return "", ""
	}
	klog.InfoS("cluster name detected; set "+noNameHint+" to choose it",
		"name", name, "source", from)
	return name, from
}

// detectClusterName reads the name from the nodes, then from the kubeadm
// ConfigMap. A failed read is a missing name, never an error: kwatch
// works without one.
func detectClusterName(
	ctx context.Context, client kubernetes.Interface,
) (name, from string) {
	if client == nil {
		return "", ""
	}
	ctx, cancel := context.WithTimeout(ctx, clusterNameTimeout)
	defer cancel()
	list, err := client.CoreV1().Nodes().List(ctx,
		metav1.ListOptions{Limit: nodeSampleLimit})
	if err != nil {
		klog.V(1).InfoS("cluster name: nodes not read", "error", err)
		return "", ""
	}
	nodes := make([]nodeMeta, 0, len(list.Items))
	for _, n := range list.Items {
		nodes = append(nodes, nodeMeta{
			Labels: n.Labels, Annotations: n.Annotations})
	}
	if name, from = clusterNameFromNodes(nodes); name != "" {
		return name, from
	}
	return kubeadmClusterName(ctx, client)
}

// kubeadmClusterName is clusterName in the ClusterConfiguration of the
// kube-system kubeadm-config ConfigMap, read with a list (the role
// grants list on ConfigMaps, not get). kubeadm's default, "kubernetes",
// is not a name.
func kubeadmClusterName(
	ctx context.Context, client kubernetes.Interface,
) (name, from string) {
	list, err := client.CoreV1().ConfigMaps("kube-system").List(ctx,
		metav1.ListOptions{FieldSelector: "metadata.name=kubeadm-config"})
	if err != nil || len(list.Items) != 1 ||
		list.Items[0].Name != "kubeadm-config" {
		return "", ""
	}
	var conf struct {
		ClusterName string `yaml:"clusterName"`
	}
	raw := list.Items[0].Data["ClusterConfiguration"]
	if yaml.Unmarshal([]byte(raw), &conf) != nil {
		return "", ""
	}
	name, ok := usableName(conf.ClusterName)
	if !ok {
		return "", ""
	}
	return name, "from the kubeadm-config ConfigMap in kube-system"
}

package delivery

import (
	"strings"

	"k8s.io/klog/v2"
)

// Paging systems and issue trackers keep one alert or issue per alert
// key. The cluster name is part of that key. Without a name, the same
// failure in two clusters that send to one account had the same key:
// the second cluster's alert was merged into the first one's, and its
// resolve closed the first one's page. kwatch then hashes the cluster's
// own identity into the key of every new incident instead (see
// incident.Config.AlertIdentity), but a name is the only identity a
// reader can tell apart, so an empty one is reported at every start.

// keyedProviders names the configured providers that track alerts or
// issues by key.
func keyedProviders(entries []providerEntry) []string {
	var names []string
	for _, entry := range entries {
		if skipsPlainMessages(entry.provider) {
			names = append(names, entry.provider.Name())
		}
	}
	return names
}

// warnUnnamedCluster reports an empty cluster name when a provider
// that tracks alerts by key is configured.
func warnUnnamedCluster(entries []providerEntry, clusterName string) {
	if strings.TrimSpace(clusterName) != "" {
		return
	}
	names := keyedProviders(entries)
	if len(names) == 0 {
		return
	}
	klog.Warningf("delivery: app.clusterName is empty and %s keep one "+
		"alert per key. Set app.clusterName on every cluster that sends "+
		"to the same account, so their alerts can be told apart and "+
		"never merge or resolve each other. Until then new alerts are "+
		"keyed by this cluster's own identity (the kube-system "+
		"namespace UID); alerts that are already open keep their key.",
		strings.Join(names, ", "))
}

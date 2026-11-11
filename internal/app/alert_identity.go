package app

import (
	"context"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

// alertIdentityTimeout bounds the one lookup made at start.
const alertIdentityTimeout = 10 * time.Second

// alertIdentity is what tells this cluster's alert keys apart when no
// cluster name is configured: the kube-system namespace UID. It is only
// hashed into alert keys and never sent anywhere. With a cluster name
// the name is part of every key already, so nothing is looked up. A
// failed lookup keeps the keys as they were before, and says so.
func alertIdentity(ctx context.Context, deps *serverDeps) string {
	name := deps.runtime.Application().ClusterName
	if strings.TrimSpace(name) != "" || deps.clients.Kubernetes == nil {
		return ""
	}
	lookupCtx, cancel := context.WithTimeout(ctx, alertIdentityTimeout)
	defer cancel()
	uid, err := kubeSystemUID(deps.clients.Kubernetes)(lookupCtx)
	if err != nil {
		klog.ErrorS(err, "app.clusterName is empty and the cluster's "+
			"own identity could not be read; alert keys of new "+
			"incidents are not unique to this cluster until the next "+
			"start. Set app.clusterName")
		return ""
	}
	return uid
}

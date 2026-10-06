package kube

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// helmReleaseSecretType is the type of the Secrets Helm stores a release
// in. They are large (the whole chart, compressed), change on every
// upgrade, and name no object a workload can reference.
const helmReleaseSecretType = "helm.sh/release.v1"

// secretSelector asks the API server to leave Helm release Secrets out,
// so they are never listed, watched or cached.
var secretSelector = fields.OneTermNotEqualSelector(
	"type", helmReleaseSecretType).String()

// secretInformer registers the cluster-wide Secret informer on factory
// with secretSelector. The factory still owns its start, transform and
// shutdown.
func secretInformer(
	factory informers.SharedInformerFactory,
) cache.SharedIndexInformer {
	return factory.InformerFor(&corev1.Secret{},
		func(c kubernetes.Interface, resync time.Duration,
		) cache.SharedIndexInformer {
			return coreinformers.NewFilteredSecretInformer(
				c, metav1.NamespaceAll, resync,
				cache.Indexers{
					cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
				},
				func(options *metav1.ListOptions) {
					options.FieldSelector = secretSelector
				})
		})
}

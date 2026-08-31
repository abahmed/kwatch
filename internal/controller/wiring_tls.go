package controller

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corev1lister "k8s.io/client-go/listers/core/v1"
)

// wireTLS wires the TLS secret informer and returns the factories it creates.
func (c *Controller) wireTLS(
	client kubernetes.Interface,
	resync time.Duration,
	scope namespaceScope,
) []informers.SharedInformerFactory {
	var tlsFactories []informers.SharedInformerFactory
	if !scope.all && len(scope.namespaces) == 0 {
		return tlsFactories
	}
	if scope.all || len(scope.namespaces) == 1 {
		opts := []informers.SharedInformerOption{
			informers.WithTweakListOptions(func(o *metav1.ListOptions) {
				o.FieldSelector = "type=kubernetes.io/tls"
				if scope.all {
					o.FieldSelector += "," + informerExcludedNamespaces(scope.forbidden)
				}
			}),
		}
		if len(scope.namespaces) == 1 {
			opts = append(opts, informers.WithNamespace(scope.namespaces[0]))
		}
		opts = append(opts, informerMemoryOptions()...)
		tf := informers.NewSharedInformerFactoryWithOptions(
			client,
			resync,
			opts...)
		tlsFactories = append(tlsFactories, tf)
		c.secretLister = tf.Core().V1().Secrets().Lister()
		c.secretsSynced = append(
			c.secretsSynced,
			tf.Core().V1().Secrets().Informer().HasSynced,
		)
	} else {
		listers := make([]corev1lister.SecretLister, 0, len(scope.namespaces))
		for _, ns := range scope.namespaces {
			ns := ns
			opts := []informers.SharedInformerOption{
				informers.WithTweakListOptions(func(o *metav1.ListOptions) {
					o.FieldSelector = "type=kubernetes.io/tls"
				}),
				informers.WithNamespace(ns),
			}
			opts = append(opts, informerMemoryOptions()...)
			tf := informers.NewSharedInformerFactoryWithOptions(
				client,
				resync,
				opts...)
			tlsFactories = append(tlsFactories, tf)
			listers = append(listers, tf.Core().V1().Secrets().Lister())
			c.secretsSynced = append(
				c.secretsSynced,
				tf.Core().V1().Secrets().Informer().HasSynced,
			)
		}
		c.secretLister = &multiSecretLister{listers: listers}
	}
	return tlsFactories
}

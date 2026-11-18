package kube

import (
	"context"
	"errors"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/metadata/metadatainformer"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/kubeclient"
	"github.com/abahmed/kwatch/internal/safego"
)

// helmIndex indexes release Secrets by "namespace/release".
const helmIndex = "helm-release"

var secretsGVR = schema.GroupVersionResource{Version: "v1",
	Resource: "secrets"}

// helmReleaseOptions select only Helm's release Secrets, by type and
// owner label. The watch is metadata-only: the API server sends labels
// and timestamps, never the release payload.
func helmReleaseOptions(options *metav1.ListOptions) {
	options.FieldSelector = fields.OneTermEqualSelector(
		"type", helmReleaseSecretType).String()
	options.LabelSelector = labels.Set{
		helmOwnerLabel: "helm"}.AsSelector().String()
}

// helmReleaseInformer builds the metadata informer of release Secrets.
func helmReleaseInformer(
	client metadata.Interface, resync time.Duration,
) (cache.SharedIndexInformer, error) {
	if client == nil {
		return nil, errors.New("no metadata client")
	}
	informer := metadatainformer.NewFilteredMetadataInformer(client,
		secretsGVR, metav1.NamespaceAll, resync, cache.Indexers{
			helmIndex: helmIndexFunc,
		}, helmReleaseOptions).Informer()
	if err := informer.SetTransform(guardTransform("helm-releases",
		trimHelmMeta)); err != nil {
		return nil, err
	}
	return informer, nil
}

// trimHelmMeta keeps what the release state needs: the cache holds
// labels and timestamps, not managed fields or annotations.
func trimHelmMeta(obj any) (any, error) {
	if m, ok := obj.(*metav1.PartialObjectMetadata); ok {
		m.ManagedFields, m.Annotations = nil, nil
	}
	return obj, nil
}

func helmIndexFunc(obj any) ([]string, error) {
	meta, ok := obj.(metav1.Object)
	if !ok {
		return nil, nil
	}
	revision, ok := HelmRevisionOf(meta)
	if !ok {
		return nil, nil
	}
	return []string{revision.Namespace + "/" + revision.Release}, nil
}

// watchHelmReleases adds the release Secret watch to the source. It is
// skipped without a metadata client or when Secrets are not watched:
// the release kind then never syncs, and the Helm check cannot verify.
func (s *Source) watchHelmReleases() error {
	if s.cfg.DisableSecrets || s.cfg.Metadata == nil {
		return nil
	}
	informer, err := helmReleaseInformer(s.cfg.Metadata, s.cfg.Resync)
	if err != nil {
		return err
	}
	handler := kubeclient.SafeEventHandler("inventory", "helm-releases",
		helmHandler(s.context, informer.GetIndexer(), s.cfg.Submit,
			s.cfg.Now))
	handle, err := informer.AddEventHandler(handler)
	if err != nil {
		return err
	}
	if err := s.track(SecretsResource, false, informer,
		handle); err != nil {
		return err
	}
	s.synced[KindHelmRelease] = handle.HasSynced
	s.helm = informer
	return nil
}

// helmHandler describes the release a changed revision Secret belongs
// to again, from every revision of it the cache holds.
func helmHandler(
	lifecycle func() context.Context, indexer cache.Indexer,
	submit Submit, now func() time.Time,
) cache.ResourceEventHandler {
	describe := func(obj any) {
		if tomb, ok := obj.(cache.DeletedFinalStateUnknown); ok {
			obj = tomb.Obj
		}
		meta, ok := obj.(metav1.Object)
		if !ok {
			return
		}
		revision, ok := HelmRevisionOf(meta)
		if !ok {
			return
		}
		submit(lifecycle(), HelmReleaseObservation(
			HelmReleaseID(revision.Namespace, revision.Release),
			cachedRevisions(indexer,
				revision.Namespace+"/"+revision.Release), now()))
	}
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    describe,
		UpdateFunc: func(_, new any) { describe(new) },
		DeleteFunc: describe,
	}
}

// cachedRevisions are the revisions of one release in the cache.
func cachedRevisions(indexer cache.Indexer, key string) []HelmRevision {
	objects, err := indexer.ByIndex(helmIndex, key)
	if err != nil {
		return nil
	}
	out := make([]HelmRevision, 0, len(objects))
	for _, obj := range objects {
		if meta, ok := obj.(metav1.Object); ok {
			if revision, ok := HelmRevisionOf(meta); ok {
				out = append(out, revision)
			}
		}
	}
	return out
}

// runHelm starts the release watch; the channel closes once it stopped
// after ctx ended.
func (s *Source) runHelm(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	if s.helm == nil {
		close(done)
		return done
	}
	safego.Go("helm-releases", func() {
		defer close(done)
		s.helm.Run(ctx.Done())
	})
	return done
}

// helmReleaseKind reports the kind the release watch serves, for the
// disabled check: it goes with Secrets.
func helmReleaseKind(kind inventory.Kind) bool {
	return kind == KindHelmRelease
}

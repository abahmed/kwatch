package dashboard

import (
	"context"
	"slices"
	"sync"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/release"
	"github.com/abahmed/kwatch/internal/storage"
)

// releasesEvery is how often the releases are looked at: a rollout's
// replicas and the incidents it causes change between two looks.
const releasesEvery = 15 * time.Second

// releasesKey is where the release book lives in the state bucket.
const releasesKey = "dashboard.releases"

// releaseBook is the book the Releases page reads; the runner writes it.
type releaseBook struct {
	mu   sync.Mutex
	book release.Book
}

// snapshot is a copy the handler may read without the lock.
func (b *releaseBook) snapshot() release.Book {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.book.Clone()
}

// OwnWorkload is kwatch's own namespace and Deployment, whose rollouts
// are kwatch restarting rather than releases.
type OwnWorkload struct {
	Namespace, Name string
}

// RunReleases keeps the release book until ctx ends: every releasesEvery
// it reads the change journal, the rollouts and the incidents, and
// saves the book when it changed. Without a state store the book lives
// in memory only. It sends nothing.
func (s *Server) RunReleases(
	ctx context.Context, store *storage.Store, own OwnWorkload,
) {
	b := &releaseBook{}
	var values *storage.Keyed[release.Book]
	if store != nil {
		v := storage.StateValues[release.Book](store)
		values = &v
		if saved, ok, err := v.Get(releasesKey); err == nil && ok {
			b.book = saved
		}
	}
	s.sources.releases.Store(b)
	defer s.sources.releases.Store(nil)
	tick := time.NewTicker(releasesEvery)
	defer tick.Stop()
	for {
		s.lookAtReleases(b, values, own)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// lookAtReleases runs one look and saves the book when it changed.
func (s *Server) lookAtReleases(
	b *releaseBook, values *storage.Keyed[release.Book], own OwnWorkload,
) {
	model := s.sources.model.Load()
	if model == nil {
		return
	}
	var items []incident.Incident
	if reader := s.sources.incidents.Load(); reader != nil {
		items = (*reader).Incidents()
	}
	view := release.View{Model: *model, Incidents: items,
		InScope: s.opts.Scope.over(*model), OwnNamespace: own.Namespace,
		OwnWorkload: own.Name, Detail: s.opts.ChangeHistory}
	b.mu.Lock()
	changed := release.Observe(&b.book, view, s.now.Now())
	saved := b.book.Clone()
	b.mu.Unlock()
	if !changed || values == nil {
		return
	}
	if err := values.Put(releasesKey, saved); err != nil {
		klog.ErrorS(err, "dashboard: release book not saved",
			"component", "dashboard-releases")
	}
}

// over is the scope as a release look reads it over model: the whole
// namespace scope when Allowed is set, so a namespace the label
// selector leaves out has no releases; the two lists otherwise.
func (sc Scope) over(model inventory.Reader) func(namespace string) bool {
	if sc.Allowed == nil {
		return sc.covers
	}
	return func(namespace string) bool {
		return sc.Allowed(model, namespace)
	}
}

// covers reports a namespace kwatch watches: every one when Watched is
// empty, never an excluded one.
func (sc Scope) covers(namespace string) bool {
	if slices.Contains(sc.Excluded, namespace) {
		return false
	}
	return len(sc.Watched) == 0 || slices.Contains(sc.Watched, namespace)
}

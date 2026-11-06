//go:build browser

package dashboard

import (
	"time"

	"github.com/abahmed/kwatch/internal/release"
)

// browserReleases is a release book with one release of each kind of
// row the issue page and Overview draw: healthy, failed with an undo command, and
// down under GitOps with an undo in Git.
func browserReleases() *releaseBook {
	at := time.Now().Add(-time.Hour)
	after := 12.0
	return &releaseBook{book: release.Book{Records: []release.Record{
		{ID: "a", Kind: "deployment", Namespace: "shop", Name: "api",
			At: at, Verdict: release.Failed, Ready: 2, Desired: 3,
			OldServing: true, Stalled: true, Deadline: true,
			Incident: "inc-001",
			Images: []release.Image{{Container: "app",
				Before: "registry.example.com/shop/api:1.4",
				After:  "registry.example.com/shop/api:1.5"}},
			Detail: &release.Detail{RestartsAfter: &after,
				ErrorLine: "panic: setting X must be set"}},
		{ID: "b", Kind: "statefulset", Namespace: "data", Name: "db",
			At: at.Add(-time.Hour), Verdict: release.Down,
			App: "argocd/data", Changed: []string{"environment"}},
		{ID: "d", Kind: "deployment", Namespace: "shop", Name: "web",
			At: at.Add(-30 * time.Minute), Verdict: release.Regressed},
		{ID: "c", Kind: "daemonset", Namespace: "ops", Name: "agent",
			At: at.Add(-2 * time.Hour), Verdict: release.Healthy,
			RolledOut: at.Add(-2*time.Hour + time.Minute),
			Ready:     5, Desired: 5},
	}}}
}

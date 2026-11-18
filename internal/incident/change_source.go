package incident

import (
	"strings"

	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
)

// SourceOf is what change records about where it came from: its GitOps
// application, repository and commit, and the first new image it set.
// Messages and the dashboard link it with format.SourceLinks.
func SourceOf(change inventory.Change) format.ChangeSource {
	src := format.ChangeSource{App: change.App, Repo: change.Repo,
		Commit: change.Commit}
	for _, f := range change.Fields {
		if strings.HasSuffix(f.Path, ".image") && f.After != "" {
			src.Image = f.After
			break
		}
	}
	return src
}

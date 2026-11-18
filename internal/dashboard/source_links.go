package dashboard

import (
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/release"
)

// SetGitOpsLinks gives the dashboard app.gitOpsLinks: the web page
// template of one application per GitOps tool. Without it a change
// names its GitOps app without a link. A nil server ignores it.
func (s *Server) SetGitOpsLinks(templates map[string]string) {
	if s == nil {
		return
	}
	if len(templates) == 0 {
		s.sources.gitOps.Store(nil)
		return
	}
	copied := make(map[string]string, len(templates))
	for tool, template := range templates {
		copied[tool] = template
	}
	s.sources.gitOps.Store(&copied)
}

// gitOpsLinks is the configured templates, nil without.
func (s *Server) gitOpsLinks() map[string]string {
	if templates := s.sources.gitOps.Load(); templates != nil {
		return *templates
	}
	return nil
}

// addChangeLinks links each change of d to where it came from, built
// only from what the objects recorded and the configured templates.
func addChangeLinks(d *IncidentDetail, templates map[string]string) {
	for i := range d.Changes {
		d.Changes[i].Links = format.SourceLinks(d.Changes[i].source,
			templates)
	}
	if d.FixedBy != nil {
		d.FixedBy.Links = format.SourceLinks(d.FixedBy.source, templates)
	}
}

// addReleaseLinks links the release of d to where it came from.
func addReleaseLinks(d *IncidentDetail, templates map[string]string) {
	if d.Release != nil {
		d.Release.Links = format.SourceLinks(
			releaseSource(d.Release.Record), templates)
	}
}

// releaseSource is what a release records of its origin, with the
// first new image.
func releaseSource(r release.Record) format.ChangeSource {
	src := format.ChangeSource{App: r.App, Repo: r.Repo, Commit: r.Commit}
	if len(r.Images) > 0 {
		src.Image = r.Images[0].After
	}
	return src
}

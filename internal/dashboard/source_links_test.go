package dashboard

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/release"
)

const linkTemplate = "https://gitops.example.com/applications/{app}"

func sourcedChange() inventory.Change {
	return inventory.Change{
		Entity: inventory.EntityID{Kind: "deployment", Namespace: "shop",
			Name: "api"},
		At: time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC), App: "argocd/shop",
		Repo: "https://github.com/example/shop", Commit: "abcdef1",
		Fields: []inventory.FieldChange{{
			Path:   "spec.template.spec.containers[app].image",
			Before: "example/api:1.4", After: "example/api:1.5"}},
	}
}

func TestChangeLinksWithSetting(t *testing.T) {
	d := IncidentDetail{Changes: []ChangeItem{
		toChangeItem(sourcedChange(), time.Time{})}}

	addChangeLinks(&d, map[string]string{"argocd": linkTemplate})

	assert.Equal(t, []format.SourceLink{
		{Label: "Argo CD app shop",
			URL: "https://gitops.example.com/applications/shop"},
		{Label: "commit abcdef1",
			URL: "https://github.com/example/shop/commit/abcdef1"},
		{Label: "image api:1.5", URL: "https://hub.docker.com/r/example/api"},
	}, d.Changes[0].Links)
}

func TestChangeLinksWithoutSettingNameTheApp(t *testing.T) {
	change := sourcedChange()
	change.Repo, change.Commit, change.Fields = "", "", nil
	d := IncidentDetail{Changes: []ChangeItem{
		toChangeItem(change, time.Time{})}}

	addChangeLinks(&d, nil)

	assert.Equal(t, []format.SourceLink{{Label: "Argo CD app shop"}},
		d.Changes[0].Links)
}

func TestChangeLinksAbsentWithoutSourceData(t *testing.T) {
	change := sourcedChange()
	change.App, change.Repo, change.Commit, change.Fields = "", "", "", nil
	d := IncidentDetail{Changes: []ChangeItem{
		toChangeItem(change, time.Time{})}}

	addChangeLinks(&d, map[string]string{"argocd": linkTemplate})

	assert.Empty(t, d.Changes[0].Links)
}

func TestReleaseLinks(t *testing.T) {
	d := IncidentDetail{Release: &ReleaseCause{Record: release.Record{
		App: "flux/apps", Repo: "https://git.example.com/team/shop",
		Commit: "v1.5.0",
		Images: []release.Image{{After: "registry.example.com/api:1.5"}},
	}}}

	addReleaseLinks(&d, map[string]string{"flux": linkTemplate})

	assert.Equal(t, []format.SourceLink{
		{Label: "Flux app apps",
			URL: "https://gitops.example.com/applications/apps"},
		{Label: "tag v1.5.0"},
		{Label: "repository", URL: "https://git.example.com/team/shop"},
	}, d.Release.Links)
}

func TestSetGitOpsLinksCopiesTheSetting(t *testing.T) {
	s := &Server{}
	templates := map[string]string{"argocd": linkTemplate}
	s.SetGitOpsLinks(templates)
	templates["argocd"] = "changed"

	assert.Equal(t, linkTemplate, s.gitOpsLinks()["argocd"])
	s.SetGitOpsLinks(nil)
	assert.Nil(t, s.gitOpsLinks())
}

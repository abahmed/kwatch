package format

import (
	"reflect"
	"testing"
)

const testTemplate = "https://gitops.example.com/applications/{app}"

func TestSourceLinksWithEverything(t *testing.T) {
	src := ChangeSource{App: "argocd/shop",
		Repo:   "https://github.com/example/shop.git",
		Commit: "1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b",
		Image:  "example/api:1.5"}
	got := SourceLinks(src, map[string]string{"argocd": testTemplate})
	want := []SourceLink{
		{Label: "Argo CD app shop",
			URL: "https://gitops.example.com/applications/shop"},
		{Label: "commit 1a2b3c4", URL: "https://github.com/example/shop" +
			"/commit/1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b"},
		{Label: "image api:1.5",
			URL: "https://hub.docker.com/r/example/api"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("links = %#v, want %#v", got, want)
	}
}

func TestSourceLinksWithoutSettingNamesAppOnly(t *testing.T) {
	got := SourceLinks(ChangeSource{App: "flux/apps"}, nil)
	want := []SourceLink{{Label: "Flux app apps"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("links = %#v, want %#v", got, want)
	}
}

func TestSourceLinksUnknownHostKeepsRevisionAsText(t *testing.T) {
	src := ChangeSource{Repo: "https://git.example.com/team/shop",
		Commit: "v1.4.0", Image: "registry.example.com/team/api:1.4"}
	got := SourceLinks(src, nil)
	want := []SourceLink{{Label: "tag v1.4.0"},
		{Label: "repository", URL: "https://git.example.com/team/shop"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("links = %#v, want %#v", got, want)
	}
}

func TestSourceLinksIgnoresInvalidValues(t *testing.T) {
	cases := map[string]ChangeSource{
		"http repository":   {Repo: "http://github.com/example/shop"},
		"credentials":       {Repo: "https://user:token@github.com/a/b"},
		"no path":           {Repo: "https://github.com"},
		"query":             {Repo: "https://github.com/a/b?x=1"},
		"other scheme":      {Repo: "git@github.com:example/shop.git"},
		"bad revision":      {Commit: "main; rm -rf /"},
		"unknown tool":      {App: "kubectl/shop"},
		"private registry":  {Image: "registry.example.com/team/api:1"},
		"docker deep path":  {Image: "docker.io/a/b/c:1"},
		"upper-case image":  {Image: "Example/API:1"},
		"empty application": {App: "argocd/"},
	}
	templates := map[string]string{"argocd": testTemplate}
	for name, src := range cases {
		if got := SourceLinks(src, templates); len(got) != 0 {
			t.Errorf("%s: links = %#v, want none", name, got)
		}
	}
}

func TestSourceLinksInvalidTemplateNamesAppWithoutLink(t *testing.T) {
	for _, template := range []string{"https://gitops.example.com",
		"ftp://gitops.example.com/{app}", "{app}",
		"https://user:pw@gitops.example.com/{app}"} {
		got := SourceLinks(ChangeSource{App: "argocd/shop"},
			map[string]string{"argocd": template})
		if len(got) != 1 || got[0].URL != "" {
			t.Errorf("template %q: links = %#v, want name only",
				template, got)
		}
	}
}

func TestCommitURLKnownHosts(t *testing.T) {
	cases := []struct{ base, rev, want string }{
		{"https://gitlab.com/group/sub/app", "abcdef1",
			"https://gitlab.com/group/sub/app/-/commit/abcdef1"},
		{"https://bitbucket.org/team/app", "v2.0.1",
			"https://bitbucket.org/team/app/src/v2.0.1"},
		{"https://github.com/team/app", "v2.0.1",
			"https://github.com/team/app/tree/v2.0.1"},
	}
	for _, c := range cases {
		got, ok := CommitURL(c.base, c.rev)
		if !ok || got != c.want {
			t.Errorf("CommitURL(%q, %q) = %q, %v; want %q",
				c.base, c.rev, got, ok, c.want)
		}
	}
	if _, ok := CommitURL("https://github.com/team", "abcdef1"); ok {
		t.Error("a host page without a repository got a commit link")
	}
}

func TestImageURLPublicRegistries(t *testing.T) {
	cases := map[string]string{
		"nginx:1.27":                  "https://hub.docker.com/_/nginx",
		"docker.io/library/redis@sha": "https://hub.docker.com/_/redis",
		"quay.io/team/app:2":          "https://quay.io/repository/team/app",
	}
	for ref, want := range cases {
		if got, ok := ImageURL(ref); !ok || got != want {
			t.Errorf("ImageURL(%q) = %q, %v; want %q", ref, got, ok, want)
		}
	}
	if _, ok := ImageURL("localhost:5000/app:1"); ok {
		t.Error("a local registry got a link")
	}
}

package format

import (
	"net/url"
	"regexp"
	"strings"
)

// SourceLink is one way to the source of a change: a GitOps
// application, a commit, a repository or an image. URL is empty when
// no safe address is known; the label is then shown as text.
type SourceLink struct {
	Label string `json:"label"`
	URL   string `json:"url,omitempty"`
}

// ChangeSource is what kwatch read about where a change came from: the
// GitOps application ("argocd/shop"), the repository and revision a
// CI or GitOps tool recorded on the object, and the new image.
type ChangeSource struct {
	App    string
	Repo   string
	Commit string
	Image  string
}

// AppPlaceholder is replaced by the application's name in a GitOps
// link template ("https://gitops.example.com/applications/{app}").
const AppPlaceholder = "{app}"

// gitOpsTools names the tools GitOpsApp prefixes an application with.
var gitOpsTools = map[string]string{
	"argocd":     "Argo CD app",
	"flux":       "Flux app",
	"helm":       "Helm release",
	"helm-chart": "Helm chart",
}

// SourceLinks lists the ways to the source of a change, application
// first. templates maps a tool ("argocd", "flux") to the link template
// of its web UI; a tool without one is named without a link. A
// revision is linked only on a repository host whose commit pages are
// known; elsewhere the repository is linked and the revision is text.
func SourceLinks(src ChangeSource, templates map[string]string) []SourceLink {
	var out []SourceLink
	if link, ok := appLink(src.App, templates); ok {
		out = append(out, link)
	}
	out = append(out, revisionLinks(src.Repo, src.Commit)...)
	if address, ok := ImageURL(src.Image); ok {
		out = append(out, SourceLink{
			Label: "image " + shortImageName(src.Image), URL: address})
	}
	return out
}

// appLink names the GitOps application, linked when its tool has a
// template.
func appLink(app string, templates map[string]string) (SourceLink, bool) {
	tool, name, ok := strings.Cut(strings.TrimSpace(app), "/")
	words, known := gitOpsTools[tool]
	if !ok || !known || name == "" {
		return SourceLink{}, false
	}
	link := SourceLink{Label: words + " " + name}
	if address, ok := GitOpsURL(templates[tool], name); ok {
		link.URL = address
	}
	return link, true
}

// GitOpsURL fills a link template with the application's name. An
// empty or invalid template gives no address.
func GitOpsURL(template, name string) (string, bool) {
	if !ValidLinkTemplate(template) || name == "" {
		return "", false
	}
	return strings.ReplaceAll(strings.TrimSpace(template), AppPlaceholder,
		url.PathEscape(name)), true
}

// ValidLinkTemplate reports a template that is an absolute http or
// https address naming the application.
func ValidLinkTemplate(template string) bool {
	template = strings.TrimSpace(template)
	if !strings.Contains(template, AppPlaceholder) {
		return false
	}
	u, err := url.Parse(strings.ReplaceAll(template, AppPlaceholder, "x"))
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") &&
		u.Host != "" && u.User == nil
}

// revisionLinks are the commit, linked when the host is known, or the
// repository and the revision as text.
func revisionLinks(repo, revision string) []SourceLink {
	base, repoOK := RepoURL(repo)
	if !ValidRevision(revision) {
		revision = ""
	}
	var out []SourceLink
	if revision != "" {
		link := SourceLink{Label: revisionLabel(revision)}
		if repoOK {
			link.URL, _ = CommitURL(base, revision)
		}
		out = append(out, link)
	}
	if repoOK && (len(out) == 0 || out[0].URL == "") {
		out = append(out, SourceLink{Label: "repository", URL: base})
	}
	return out
}

// shaPattern is an abbreviated or full Git object name.
var shaPattern = regexp.MustCompile(`^[0-9a-f]{7,64}$`)

// tagPattern is a tag or other ref name kwatch quotes and links: no
// spaces, no path, no leading dot or dash.
var tagPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,99}$`)

// ValidRevision reports a commit SHA or a tag.
func ValidRevision(revision string) bool {
	return shaPattern.MatchString(revision) ||
		tagPattern.MatchString(revision)
}

// isSHA reports a revision that is a commit SHA rather than a tag.
func isSHA(revision string) bool {
	return shaPattern.MatchString(revision)
}

// revisionLabel is "commit 1a2b3c4" or "tag v1.4.0".
func revisionLabel(revision string) string {
	if isSHA(revision) {
		return "commit " + revision[:7]
	}
	return "tag " + revision
}

// maxRepoURL bounds a repository address kwatch keeps.
const maxRepoURL = 300

// RepoURL is the repository's web address: an https address with a
// host and a path, without credentials, query or fragment, and without
// a trailing ".git" or slash, at most maxRepoURL bytes. Anything else is
// refused.
func RepoURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || len(raw) > maxRepoURL || u.Scheme != "https" ||
		u.Host == "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		u.Opaque != "" || strings.ContainsAny(raw, " \"'<>\\") {
		return "", false
	}
	path := strings.TrimSuffix(strings.TrimSuffix(u.Path, "/"), ".git")
	if strings.Trim(path, "/") == "" {
		return "", false
	}
	return "https://" + strings.ToLower(u.Host) + path, true
}

// commitPaths are the commit and tag pages of the repository hosts
// whose address layout is public and stable. Any other host, a
// self-hosted one included, gets no constructed address.
var commitPaths = map[string][2]string{
	"github.com":    {"/commit/", "/tree/"},
	"gitlab.com":    {"/-/commit/", "/-/tree/"},
	"bitbucket.org": {"/commits/", "/src/"},
}

// CommitURL is the page of revision in the repository at base (from
// RepoURL), on a known host only.
func CommitURL(base, revision string) (string, bool) {
	u, err := url.Parse(base)
	if err != nil || !ValidRevision(revision) {
		return "", false
	}
	paths, ok := commitPaths[u.Host]
	if !ok || strings.Count(strings.Trim(u.Path, "/"), "/") < 1 {
		return "", false
	}
	path := paths[1]
	if isSHA(revision) {
		path = paths[0]
	}
	return base + path + url.PathEscape(revision), true
}

package dashboard

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/version"
)

// docsRefMark stands for the git ref of the docs in the shell. The sign-in
// page links to the docs of the running version, not to the main branch,
// which may describe a newer release.
const docsRefMark = "{{docs-ref}}"

// releaseTag matches a release version such as 1.0.0, v1.0.0 or
// v1.0.0-rc.7. A build that sits past a tag ("v1.0.0-3-gabc1234") or is
// dirty has no tag of its own, so it does not match.
var releaseTag = regexp.MustCompile(
	`^v?\d+\.\d+\.\d+(-[0-9A-Za-z]+(\.[0-9A-Za-z]+)*)?$`)

var pastTag = regexp.MustCompile(`-\d+-g[0-9a-f]+|-dirty$`)

// docsRef returns the git ref whose docs match v: the release tag for a
// release build, "main" for a dev or untagged build.
func docsRef(v string) string {
	v = strings.TrimSpace(v)
	if !releaseTag.MatchString(v) || pastTag.MatchString(v) {
		return "main"
	}
	return "v" + strings.TrimPrefix(v, "v")
}

// withDocsRef fills docsRefMark in body for the running version.
func withDocsRef(body []byte) []byte {
	return bytes.ReplaceAll(body, []byte(docsRefMark),
		[]byte(docsRef(version.Short())))
}

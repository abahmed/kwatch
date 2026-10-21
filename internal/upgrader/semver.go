package upgrader

import (
	"regexp"
	"strconv"
	"strings"
)

// releaseTagRe is the only tag shape the update notice accepts:
// vMAJOR.MINOR.PATCH with an optional pre-release and build part. The
// GitHub API is outside kwatch's control, and the tag is sent to every
// configured provider.
var releaseTagRe = regexp.MustCompile(
	`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)` +
		`(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?` +
		`(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$`)

// maxTagLength bounds a tag before it is matched or announced.
const maxTagLength = 64

func validReleaseTag(tag string) bool {
	return len(tag) <= maxTagLength && releaseTagRe.MatchString(tag)
}

// semver is a parsed release: the core numbers and the pre-release
// identifiers (empty for a release). Build metadata does not count.
type semver struct {
	core [3]int
	pre  []string
}

// parseVersion reads "v1.2.3" or "1.2.3" with an optional "-pre" and
// "+build" part. ok is false for anything else.
func parseVersion(v string) (semver, bool) {
	v = strings.TrimSpace(v)
	if !validReleaseTag(v) {
		return semver{}, false
	}
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	var out semver
	if i := strings.IndexByte(v, '-'); i >= 0 {
		out.pre = strings.Split(v[i+1:], ".")
		v = v[:i]
	}
	for i, f := range strings.Split(v, ".") {
		out.core[i], _ = strconv.Atoi(f)
	}
	return out, true
}

// compare orders two versions by semantic versioning: the core numbers
// first, then a release before any of its pre-releases, then the
// pre-release identifiers one by one (numbers by value and before words,
// words by text, a shorter list before a longer one that starts with it).
func (a semver) compare(b semver) int {
	for i := range a.core {
		if a.core[i] != b.core[i] {
			return sign(a.core[i] - b.core[i])
		}
	}
	switch {
	case len(a.pre) == 0 && len(b.pre) == 0:
		return 0
	case len(a.pre) == 0:
		return 1
	case len(b.pre) == 0:
		return -1
	}
	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		if c := compareIdentifier(a.pre[i], b.pre[i]); c != 0 {
			return c
		}
	}
	return sign(len(a.pre) - len(b.pre))
}

func compareIdentifier(a, b string) int {
	an, aerr := strconv.Atoi(a)
	bn, berr := strconv.Atoi(b)
	switch {
	case aerr == nil && berr == nil:
		return sign(an - bn)
	case aerr == nil:
		return -1
	case berr == nil:
		return 1
	}
	return strings.Compare(a, b)
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}

// isNewer reports whether latest is a strictly newer release than current.
// A plain string comparison would nag a v0.12.0 install about v0.9.5, and a
// build running ahead of the latest published release about that release.
// When either side is not a version (a development or e2e build, an
// unusual tag) nothing says the release is newer, so nothing is reported.
func isNewer(latest, current string) bool {
	l, lok := parseVersion(latest)
	c, cok := parseVersion(current)
	return lok && cok && l.compare(c) > 0
}

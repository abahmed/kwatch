package format

import (
	"regexp"
	"strings"
)

// imagePathPattern is a registry repository path: lower-case parts
// split by slashes, as the distribution spec allows.
var imagePathPattern = regexp.MustCompile(
	`^[a-z0-9]+(?:[._-][a-z0-9]+)*(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)*$`)

// dockerHubHosts are the names the public Docker registry goes by.
var dockerHubHosts = map[string]bool{
	"docker.io": true, "index.docker.io": true,
	"registry-1.docker.io": true,
}

// ImageURL is the web page of an image's repository on a public
// registry whose page addresses are known (Docker Hub, Quay). Any
// other registry gets no address: the image is shown as text.
func ImageURL(ref string) (string, bool) {
	host, path, ok := splitImage(ref)
	if !ok || !imagePathPattern.MatchString(path) {
		return "", false
	}
	switch {
	case dockerHubHosts[host]:
		if owner, name, found := strings.Cut(path, "/"); found {
			if owner == "library" && !strings.Contains(name, "/") {
				return "https://hub.docker.com/_/" + name, true
			}
			if strings.Contains(name, "/") {
				return "", false
			}
			return "https://hub.docker.com/r/" + path, true
		}
		return "https://hub.docker.com/_/" + path, true
	case host == "quay.io" && strings.Count(path, "/") == 1:
		return "https://quay.io/repository/" + path, true
	}
	return "", false
}

// splitImage splits an image reference into its registry host and
// repository path, without tag or digest. A reference without a host
// is on Docker Hub.
func splitImage(ref string) (host, path string, ok bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.ContainsAny(ref, " \t\"'<>") {
		return "", "", false
	}
	ref, _, _ = strings.Cut(ref, "@")
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		ref = ref[:i]
	}
	first, rest, found := strings.Cut(ref, "/")
	if found && (strings.ContainsAny(first, ".:") || first == "localhost") {
		return strings.ToLower(first), rest, rest != ""
	}
	return "docker.io", ref, true
}

// shortImageName is an image without its registry and path:
// "registry.io/team/api:1.2" is "api:1.2"; a digest is kept short.
func shortImageName(ref string) string {
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		ref = ref[i+1:]
	}
	if name, digest, ok := strings.Cut(ref, "@sha256:"); ok &&
		len(digest) > 12 {
		return name + "@sha256:" + digest[:12]
	}
	return ref
}

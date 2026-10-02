package rootcause

import "strings"

// RegistryHost returns the registry of an image reference; references
// without a host come from Docker Hub.
func RegistryHost(image string) string {
	first, _, found := strings.Cut(image, "/")
	if !found || (!strings.ContainsAny(first, ".:") && first != "localhost") {
		return "docker.io"
	}
	return first
}

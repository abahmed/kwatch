package explain

import "github.com/abahmed/kwatch/internal/format"

// closestName is format.ClosestName: the one of names a very small
// number of edits away from missing, or "" when none or two are.
func closestName(missing string, names []string) string {
	return format.ClosestName(missing, names)
}

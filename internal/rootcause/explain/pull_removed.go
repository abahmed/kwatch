package explain

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// pullRemoved is an image the registry no longer serves: the same
// reference fails with "manifest unknown" for workloads that have
// nothing else in common. One workload failing is a typo or a bad tag
// of its own; several is the registry having dropped it.
const pullRemoved detection.Mode = "Removed"

// RemovedImageWorkloads is how many workloads must fail to pull the
// same image for the registry to be blamed for dropping it.
const RemovedImageWorkloads = 3

// pulledImage finds the image a failed pull names.
var pulledImage = regexp.MustCompile(`failed to pull image "([^"]+)"`)

// removedImage reports a pull whose image failed with an image-level
// answer for at least RemovedImageWorkloads workloads.
func (v *view) removedImage(text string) bool {
	match := pulledImage.FindStringSubmatch(strings.ToLower(text))
	if match == nil || !imageErrorPattern.MatchString(
		strings.ToLower(text)) {
		return false
	}
	image := match[1]
	if n, ok := v.removedImages[image]; ok {
		return n >= RemovedImageWorkloads
	}
	owners := map[inventory.EntityID]bool{}
	for id := range v.s.Findings {
		other, ok := v.pullText(id)
		if ok && strings.Contains(strings.ToLower(other),
			`pull image "`+image+`"`) {
			owners[v.workloadOf(id)] = true
		}
	}
	if v.removedImages == nil {
		v.removedImages = map[string]int{}
	}
	v.removedImages[image] = len(owners)
	return len(owners) >= RemovedImageWorkloads
}

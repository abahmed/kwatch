package compose

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// volumeSizeWords say how much of a claim is used: "19 GiB of 20 GiB".
// The detector writes it as "19.2Gi of 20Gi".
func volumeSizeWords(s detection.Finding) string {
	size := evidence(s, detection.EvidenceVolumeSize)
	used, capacity, ok := strings.Cut(size, " of ")
	if !ok {
		return ""
	}
	return humanBytes(used) + " of " + humanBytes(capacity)
}

// usedOfWords says a share used with the size it is of: "98% of 20
// GiB" when the used bytes round to the whole, else "96% (48 GiB of 50
// GiB)".
func usedOfWords(pct string, s detection.Finding) string {
	size := evidence(s, detection.EvidenceVolumeSize)
	used, capacity, ok := strings.Cut(size, " of ")
	if !ok {
		return pct
	}
	if strings.TrimPrefix(humanBytes(used), "about ") == humanBytes(capacity) {
		return pct + " of " + humanBytes(capacity)
	}
	return pct + " (" + humanBytes(used) + " of " + humanBytes(capacity) + ")"
}

// volumeUserSentences name the workloads that mount a nearly full
// claim, so a person knows who is hurt by it: "It is used by postgres
// and reporter."
func volumeUserSentences(s detection.Finding) []sentence {
	users := evidence(s, detection.EvidenceVolumeUsedBy)
	if users == "" {
		return nil
	}
	return []sentence{{part: partConsequence, weight: 1,
		text: "It is used by " + andList(users) + "."}}
}

// volumeInodeSentences say how many inodes a claim has used beside its
// bytes: "Its inodes are 99% used, its space 12 GiB of 100 GiB." Files
// fail to be created with the bytes mostly free.
func volumeInodeSentences(f caseFacts) []sentence {
	var out []sentence
	for _, s := range f.members {
		inodes := evidence(s, detection.EvidenceVolumeInodes)
		if inodes == "" {
			continue
		}
		text := "Its inodes are " + inodes + " used"
		if size := volumeSizeWords(s); size != "" {
			text += ", its space " + size
		}
		out = append(out, sentence{part: partProof, weight: weightUsage,
			text: endSentence(text)})
		out = append(out, volumeUserSentences(s)...)
	}
	return out
}

// andList writes "a, b" as "a and b" and "a, b, c" as "a, b and c".
// A "+N more" tail is left as it is.
func andList(names string) string {
	if strings.Contains(names, " more") {
		return names
	}
	i := strings.LastIndex(names, ", ")
	if i < 0 {
		return names
	}
	return names[:i] + " and " + names[i+2:]
}

// diskFullText matches a write that failed because the disk is full.
var diskFullText = regexp.MustCompile(
	`(?i)no space left on device|disk quota exceeded|enospc`)

// volumeCrashSentences quote the crash that names a full disk, beside a
// claim that is nearly full: "postgres crashes with "PANIC: could not
// write to file: No space left on device"." The line is quoted as the
// application wrote it.
func volumeCrashSentences(f caseFacts) []sentence {
	if !f.ok || f.lead.Entity.Kind != kube.KindPVC {
		return nil
	}
	for _, s := range f.members {
		line := evidence(s, detection.EvidenceError, "message")
		if s.Entity.Kind == kube.KindPVC || !diskFullText.MatchString(line) {
			continue
		}
		return []sentence{{part: partProof, weight: weightUsage + 0.1,
			text: crashSubject(f, s.Entity) + " fails with " +
				quoted(line) + "."}}
	}
	return nil
}

// crashSubject names the workload of a crashing container when the
// incident lists it, else the container as it is.
func crashSubject(f caseFacts, id inventory.EntityID) string {
	if owner, ok := ownerIn(f.p.Impact, id); ok {
		return nameFrom(f.p.Root, owner)
	}
	return nameFrom(f.p.Root, id)
}

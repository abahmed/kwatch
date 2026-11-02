package compose

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

// groupedWith is the bad rollout with n extra findings of mode.
func groupedWith(
	p incident.Incident, n int, mode detection.Mode, advisory bool,
) incident.Incident {
	for i := range n {
		f := detection.Finding{Mode: mode, Advisory: advisory,
			Reason: "R" + string(mode),
			Entity: inventory.CoreID(kube.KindContainer, "shop",
				fmt.Sprintf("%s-%d/app", strings.ToLower(string(mode)), i)),
			Severity: detection.Warning, Since: at(3, 0),
			Summary: "extra"}
		p.Members[f.Key()] = f
	}
	return p
}

func groupedLine(msg string) string {
	for _, l := range strings.Split(msg, "\n") {
		if _, rest, ok := strings.Cut(l, groupedPrefix); ok {
			return groupedPrefix + strings.SplitN(rest, ". ", 2)[0]
		}
	}
	return ""
}

func groupedNote(p incident.Incident) string {
	return Writer{}.Write(announce(p), at(7, 0)).Note
}

func TestGroupedLineIsOmittedWhenNothingElseIsGrouped(t *testing.T) {
	assert.NotContains(t, groupedNote(badRollout()), groupedPrefix)
}

func TestGroupedLineCountsTypesHighestFirstThenAlphabetical(t *testing.T) {
	p := groupedWith(badRollout(), 3, detection.ModeProbeReadiness, false)
	p = groupedWith(p, 2, detection.ModeRolloutStuck, false)
	p = groupedWith(p, 2, detection.ModeOOMKilled, false)
	p = groupedWith(p, 1, detection.ModeImagePull, false)

	assert.Equal(t, "Also grouped: 3 probe failures, 2 out-of-memory "+
		"kills, 2 rollout stalls, 1 image pull failure",
		groupedLine(groupedNote(p)))
}

func TestGroupedLineSkipsTheRootsFindingAndAdvisories(t *testing.T) {
	p := groupedWith(badRollout(), 1, detection.ModeProbe, false)
	p = groupedWith(p, 2, detection.ModeRiskSingleReplica, true)

	assert.Equal(t, "Also grouped: 1 probe failure",
		groupedLine(groupedNote(p)))
}

func TestGroupedLineNamesFourTypesThenCountsTheRest(t *testing.T) {
	p := badRollout()
	for i, mode := range []detection.Mode{detection.ModeProbe,
		detection.ModeOOMKilled, detection.ModeImagePull,
		detection.ModeUnschedulable, detection.ModeNoEndpoints,
		detection.ModeRolloutStuck} {
		p = groupedWith(p, 10-i, mode, false)
	}

	line := groupedLine(groupedNote(p))

	assert.Equal(t, "Also grouped: 10 probe failures, 9 out-of-memory "+
		"kills, 8 image pull failures, 7 scheduling failures, and 2 more "+
		"types", line)
}

func TestGroupedLineNeverShowsARawIdentifier(t *testing.T) {
	p := groupedWith(badRollout(), 2, detection.ModeNoEndpoints, false)

	line := groupedLine(groupedNote(p))

	assert.Equal(t, "Also grouped: 2 no endpoints alerts", line)
	assert.NotContains(t, line, "NoEndpoints")
}

func TestGroupedLineIsPlainText(t *testing.T) {
	p := groupedWith(badRollout(), 2, detection.ModeProbe, false)

	line := groupedLine(groupedNote(p))

	assert.NotContains(t, line, "*")
	assert.NotContains(t, line, "`")
	assert.Equal(t, line, strings.ToValidUTF8(line, ""))
	for _, r := range line {
		assert.Less(t, r, rune(0x2000), "no emoji: %q", line)
	}
}

// The line is kept in Community and Pro alike and sits above the
// command, so the message still ends on what to run.
func TestGroupedLineShowsInCommunityAndPro(t *testing.T) {
	p := groupedWith(badRollout(), 2, detection.ModeProbe, false)
	full, community, _ := communityWriters()

	for name, w := range map[string]Writer{"pro": full,
		"community": community} {
		msg := w.Write(announce(p), at(7, 0))
		require.Contains(t, msg.Note, groupedPrefix+"2 probe failures",
			name)
		lines := strings.Split(msg.Note, "\n")
		at := slices.IndexFunc(lines, func(l string) bool {
			return strings.HasPrefix(l, groupedPrefix)
		})
		require.Greater(t, at, 0, name)
		assert.Contains(t, lines[at+1], "kubectl", name)
		at = slices.IndexFunc(msg.Doc, func(b notification.Block) bool {
			return strings.HasPrefix(b.Text(), groupedPrefix)
		})
		require.Greater(t, at, 0, name)
		assert.Equal(t, notification.CodeBlock, msg.Doc[at+2].Kind, name)
	}
}

// A provider's length cut drops the grouped line like the other lines
// above the fix; the fix is never cut.
func TestGroupedLineIsCutBeforeTheFix(t *testing.T) {
	p := groupedWith(badRollout(), 2, detection.ModeProbe, false)
	msg := Writer{}.Write(announce(p), at(7, 0))
	same := func(s string) string { return s }
	d := notification.Dialect{Text: same, Bold: same, Italic: same,
		Code: same, Fence: same, Bullet: "- ", Gap: "\n\n"}
	full := msg.Render(d)
	require.Contains(t, full, groupedPrefix)

	pinned := 0
	for _, b := range msg.Doc {
		if b.Pinned {
			pinned++
		}
	}
	require.Equal(t, 2, pinned)
	limit := len(full) - len(groupedPrefix+"2 probe failures") - 1
	cut := msg.RenderWithin(d, limit)

	assert.LessOrEqual(t, len(cut), limit)
	assert.NotContains(t, cut, groupedPrefix)
	assert.Contains(t, cut, "kubectl rollout undo")
}

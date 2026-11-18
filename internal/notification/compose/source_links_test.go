package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
)

const sourceSHA = "1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b"

// sourcedRelease is a release blamed for a failure whose change records
// its GitOps application and the commit it was built from.
func sourcedRelease(change inventory.Change) incident.Incident {
	p := createdWorkload("deployment", reasons.CrashLoopBackOff,
		2*time.Minute)
	change.Entity = p.Root
	change.At = writerNow.Add(-10 * time.Minute)
	p.Cause.Change = &change
	return p
}

// sourceLine is the message's source line, "" without one.
func sourceLine(msg notification.Message) string {
	for _, b := range msg.Doc {
		if text := b.Text(); strings.HasPrefix(text, sourceLabel) {
			return text
		}
	}
	return ""
}

func TestWriteLinksABlamedChangeToItsSource(t *testing.T) {
	p := sourcedRelease(inventory.Change{App: "argocd/shop",
		Repo: "https://github.com/example/shop", Commit: sourceSHA})
	w := Writer{GitOpsLinks: map[string]string{
		"argocd": "https://gitops.example.com/applications/{app}"}}

	msg := w.Write(announce(p), writerNow)

	assert.Equal(t, "Source: Argo CD app shop: "+
		"https://gitops.example.com/applications/shop · commit 1a2b3c4: "+
		"https://github.com/example/shop/commit/"+sourceSHA,
		sourceLine(msg))
	assert.NotContains(t, msg.Note, "https://")
}

func TestWriteWithoutSettingNamesAppNextToCommitLink(t *testing.T) {
	p := sourcedRelease(inventory.Change{App: "argocd/shop",
		Repo: "https://github.com/example/shop", Commit: "v1.4.0"})

	msg := Writer{}.Write(announce(p), writerNow)

	line := sourceLine(msg)
	assert.Contains(t, line, "Argo CD app shop · tag v1.4.0: ")
	assert.NotContains(t, line, "gitops.example.com")
}

func TestWriteAddsNoSourceLineWithoutAnAddress(t *testing.T) {
	cases := map[string]inventory.Change{
		"nothing recorded":    {},
		"app without setting": {App: "flux/apps"},
		"revision on an unknown host as text only": {
			Commit: "v1.4.0"},
	}
	for name, change := range cases {
		msg := Writer{}.Write(announce(sourcedRelease(change)), writerNow)
		assert.Empty(t, sourceLine(msg), name)
	}
}

func TestCommunityMessageHasNoSourceLine(t *testing.T) {
	p := sourcedRelease(inventory.Change{
		Repo: "https://github.com/example/shop", Commit: sourceSHA})
	w := Writer{Community: true}

	msg := w.Write(announce(p), writerNow)

	assert.Empty(t, sourceLine(msg))
}

func TestWriteLinksTheFirstRecentChangeWithoutACause(t *testing.T) {
	p := sourcedRelease(inventory.Change{})
	p.Cause = nil
	d := announce(p)
	d.Facts.Changes = []inventory.Change{{Entity: p.Root,
		At: writerNow.Add(-5 * time.Minute), Actor: "ci",
		Repo: "https://gitlab.com/team/shop", Commit: "abcdef1"}}

	msg := Writer{}.Write(d, writerNow)

	require.NotEmpty(t, sourceLine(msg))
	assert.Contains(t, sourceLine(msg),
		"https://gitlab.com/team/shop/-/commit/abcdef1")
}

package app

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOverlayNoticeTextIsFixedAndNamesWhatRuns(t *testing.T) {
	last := overlayNoticeText(overlayOutcome{invalid: true, lastGood: true})
	mounted := overlayNoticeText(overlayOutcome{invalid: true})

	require.Contains(t, last, "last one that worked")
	require.Contains(t, mounted, "mounted configuration only")
	require.Contains(t, mounted, "silences and filters")
	for _, text := range []string{last, mounted} {
		require.Contains(t, text, "the kwatch log says why")
	}
}

func TestOverlayNoticeIsSentOncePerRefusedSpec(t *testing.T) {
	disk := diskState{store: openTestStore(t)}
	var log noticeLog
	refused := overlayOutcome{invalid: true, rejected: "abc"}

	for range 3 {
		noticeOverlay(disk, refused, log.send)
	}
	require.Len(t, log.msgs, 1, "once across restarts")

	refused.rejected = "def"
	noticeOverlay(disk, refused, log.send)
	require.Len(t, log.msgs, 2, "a different refused spec is new")

	// A KwatchConfig that applies cleanly clears the record, so the same
	// refusal later is told again.
	noticeOverlay(disk, overlayOutcome{}, log.send)
	noticeOverlay(disk, refused, log.send)
	require.Len(t, log.msgs, 3)
}

func TestOverlayNoticeSaysNothingWhenTheOverlayApplied(t *testing.T) {
	disk := diskState{store: openTestStore(t)}
	var log noticeLog
	noticeOverlay(disk, overlayOutcome{}, log.send)
	require.Empty(t, log.msgs)
}

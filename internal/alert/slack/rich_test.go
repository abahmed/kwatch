package slack

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestSlackIncidentIsFormattedMrkdwn(t *testing.T) {
	text := firstText(noteBlocks(providertest.Rich()))
	require.Equal(t, "🔴 *payments* in *shop* is crash-looping.\n"+
		"It said `boom *x* @​channel`.\n"+
		"Service *web* can't serve traffic. Run\n"+
		"```\nkubectl logs payments -n shop\n```", text)
}

func TestSlackRootEditKeepsBlocksUnderNewMarker(t *testing.T) {
	root := providertest.Rich()
	got := withStatus(root, providertest.Resolve())
	text := firstText(noteBlocks(got))
	require.Contains(t, text, "✅ *payments* in *shop*")
	require.Contains(t, firstText(noteBlocks(root)), "🔴 *payments*")
}

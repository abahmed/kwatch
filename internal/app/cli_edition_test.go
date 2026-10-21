package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/feature"
)

// A command reads the state file once: copying it again for every question
// is disk pressure on a nearly full volume. With the file gone, the value
// still answers every question from what it read.
func TestCLIEditionAnswersEveryQuestionFromOneRead(t *testing.T) {
	path := stateFileWith(t, feature.ProEdition, stateNow.Add(48*time.Hour))
	edition := NewCLIEdition(path, stateNow)
	require.NoError(t, os.Remove(path))
	cfg := alertsOf("discord", "slack")

	require.Empty(t, edition.CommunitySends(cfg), "Pro keeps both")
	require.Empty(t, edition.Notices(cfg))
	require.Empty(t, edition.InactiveProFeatures(cfg))
	require.Len(t, edition.ConfigFor(cfg).Alert, 2)
}

func TestCLIEditionKeepsOneProviderForTheCommunityEdition(t *testing.T) {
	path := stateFileWith(t, feature.CommunityEdition, time.Time{})
	edition := NewCLIEdition(path, stateNow)
	cfg := alertsOf("discord", "slack")

	require.Equal(t, "discord (inactive: slack)", edition.CommunitySends(cfg))
	require.Contains(t, edition.Notices(cfg),
		"Community sends to: discord (inactive: slack)")
	require.Contains(t, edition.InactiveProFeatures(cfg),
		"alert providers beyond 1 (1 inactive: slack)")
	require.Len(t, edition.ConfigFor(cfg).Alert, 1)
}

func TestCLIEditionReadsTheInstalledEditionAtTheGivenTime(t *testing.T) {
	path := stateFileWith(t, feature.ProEdition, stateNow.Add(time.Hour))
	cfg := alertsOf("discord", "slack")

	inGrace := NewCLIEdition(path, stateNow)
	pastGrace := NewCLIEdition(path, stateNow.Add(400*24*time.Hour))

	require.Empty(t, inGrace.CommunitySends(cfg))
	require.NotEmpty(t, pastGrace.CommunitySends(cfg),
		"the injected time, not the wall clock, decides the grace")
}

func TestCLIEditionOfAMissingStateFileIsCommunity(t *testing.T) {
	edition := NewCLIEdition(filepath.Join(t.TempDir(), "none.db"), stateNow)

	require.NotEmpty(t, edition.CommunitySends(alertsOf("discord", "slack")))
	require.NotEmpty(t, CLIEdition{}.CommunitySends(
		alertsOf("discord", "slack")), "the zero value is Community")
}

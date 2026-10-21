package app

import (
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/feature"
)

// CLIEdition is the edition a command run beside the leader sees. Reading
// it copies the whole state file, which can be large on a nearly full
// volume, so a command reads it once with NewCLIEdition and passes the
// value down. The zero value reads as the Community edition.
type CLIEdition struct {
	entitlements feature.Entitlements
}

// NewCLIEdition reads the edition of the installation whose state file is
// at statePath, as it stands at now. It reads the file once.
func NewCLIEdition(statePath string, now time.Time) CLIEdition {
	return CLIEdition{entitlements: EffectiveEntitlements(statePath, now)}
}

// InactiveProFeatures names the Pro features cfg sets that this edition
// does not run. The lint command reports them.
func (c CLIEdition) InactiveProFeatures(cfg *config.Config) []string {
	return inactivePro(c.entitlements, cfg)
}

// Notices words what this edition does not run, for `kwatch lint`: the
// Pro detector families and the provider kept.
func (c CLIEdition) Notices(cfg *config.Config) []string {
	return newEditionNotice(c.entitlements, cfg).lines()
}

// ConfigFor returns cfg restricted to this edition, as startup would.
// `kwatch lint` and `kwatch test-alert` build their providers from it, so
// they report the providers that would really receive alerts: the
// Community edition keeps one.
func (c CLIEdition) ConfigFor(cfg *config.Config) *config.Config {
	return restrictConfig(cfg, c.entitlements)
}

// CommunitySends words the provider the Community edition keeps and the
// ones it leaves inactive ("slack (inactive: pagerduty)"); empty when
// nothing is capped.
func (c CLIEdition) CommunitySends(cfg *config.Config) string {
	return communitySends(c.entitlements, cfg)
}

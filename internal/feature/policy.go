package feature

import "time"

// Policy is the entitlement decision supplied by the composition root. A
// config file can request or disable capabilities, but it cannot grant a tier
// that the policy does not contain.
type Policy struct {
	Tier      Tier
	Source    string
	ExpiresAt time.Time
}

// CommunityPolicy is the safe default for every build and installation.
func CommunityPolicy() Policy {
	return Policy{Tier: Community, Source: "community"}
}

// ProPolicy is intentionally a small future seam for the license resolver.
// License validation is not implemented here; callers must only construct it
// after validating a license and its expiry.
func ProPolicy(expiresAt time.Time) Policy {
	return Policy{Tier: Pro, Source: "pro", ExpiresAt: expiresAt}
}

func (p Policy) Allows(definition Definition, now time.Time) bool {
	if p.ExpiresAt.IsZero() == false && !now.Before(p.ExpiresAt) {
		return false
	}
	return definition.Tier <= p.Tier
}

package feature

// OrCommunity returns e, or the Community entitlements when e is nil.
// Every consumer resolves a missing value through it, so a wiring that
// forgets to pass entitlements fails closed instead of giving Pro away.
func OrCommunity(e Entitlements) Entitlements {
	if e == nil {
		return CommunityEntitlements()
	}
	return e
}

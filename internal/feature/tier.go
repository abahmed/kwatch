package feature

// Tier is the product edition that includes a capability.
type Tier string

const (
	Community Tier = "community"
	Pro       Tier = "pro"
)

// tiers lists the Community capabilities. Every other catalog ID is Pro.
// The root-cause engine runs in Community; its detail is gated elsewhere.
var tiers = map[ID]Tier{
	PodDetection:  Community,
	PodScheduling: Community,
	PodOOM:        Community,
	PodRestarts:   Community,
	NodeDetection: Community,
	RootCause:     Community,
	NoiseControl:  Community,
	Scope:         Community,
	RBACAudit:     Community,
}

// TierOf returns the tier of a capability. Unlisted IDs are Pro.
func TierOf(id ID) Tier {
	if tier, ok := tiers[id]; ok {
		return tier
	}
	return Pro
}

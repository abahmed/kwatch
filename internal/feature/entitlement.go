package feature

import "sort"

// Edition names the product edition an installation runs.
type Edition string

const (
	CommunityEdition Edition = "community"
	ProEdition       Edition = "pro"
)

// Limits are the quantitative entitlements of an edition. Zero means
// unlimited for MaxProviders and MaxReplicas.
type Limits struct {
	MaxProviders    int
	RootCauseDetail bool
	MaxReplicas     int
	RetentionDays   int
	StatusJSON      bool
	Metrics         bool
	ProHints        bool
	Dashboard       bool
}

// Entitlements answers what an installation may use.
type Entitlements interface {
	Has(ID) bool
	Edition() Edition
	Limits() Limits
}

type entitlements struct {
	edition Edition
	limits  Limits
}

func (e entitlements) Has(id ID) bool {
	if e.edition == ProEdition {
		return true
	}
	return TierOf(id) == Community
}

func (e entitlements) Edition() Edition { return e.edition }
func (e entitlements) Limits() Limits   { return e.limits }

// CommunityEntitlements grants only Community-tier capabilities.
func CommunityEntitlements() Entitlements {
	return entitlements{CommunityEdition, Limits{
		MaxProviders: 1, MaxReplicas: 1, ProHints: true,
	}}
}

// ProEntitlements grants every capability.
func ProEntitlements() Entitlements {
	return entitlements{ProEdition, Limits{
		RootCauseDetail: true, RetentionDays: 90, StatusJSON: true,
		Metrics: true, Dashboard: true,
	}}
}

// Inactive returns the configured IDs that e does not entitle, sorted.
func Inactive(e Entitlements, configured []ID) []ID {
	var result []ID
	for _, id := range configured {
		if !e.Has(id) {
			result = append(result, id)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

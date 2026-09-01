package feature

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Overrides are operator-level disables. Enabled is deliberately absent:
// turning on a capability cannot safely create a monitor whose parent config
// is disabled. Monitor configuration remains the source of activation while
// entitlement decides whether an activated capability is allowed.
type Overrides struct {
	Disabled []ID
}

type Decision struct {
	ID          ID     `json:"id"`
	Description string `json:"description"`
	Tier        string `json:"tier"`
	Requested   bool   `json:"requested"`
	Allowed     bool   `json:"allowed"`
	Enabled     bool   `json:"enabled"`
	Reason      string `json:"reason,omitempty"`
	Lifecycle   string `json:"lifecycle"`
}

// Plan is immutable after Build. Components should receive a Plan (or a
// small derived policy) instead of checking product tier themselves.
type Plan struct {
	PolicySource string          `json:"policySource"`
	Tier         string          `json:"tier"`
	ExpiresAt    time.Time       `json:"expiresAt,omitempty"`
	Decisions    map[ID]Decision `json:"decisions"`
}

func Build(policy Policy, requested map[ID]bool, overrides Overrides, now time.Time) (Plan, error) {
	if err := ValidateCatalog(); err != nil {
		return Plan{}, err
	}
	known := make(map[ID]Definition, len(definitions))
	for _, definition := range definitions {
		known[definition.ID] = definition
	}
	if err := validateIDs(overrides.Disabled, known); err != nil {
		return Plan{}, err
	}
	disabled := make(map[ID]bool, len(overrides.Disabled))
	for _, id := range overrides.Disabled {
		disabled[id] = true
	}

	plan := Plan{
		PolicySource: policy.Source,
		Tier:         tierName(policy.Tier),
		ExpiresAt:    policy.ExpiresAt,
		Decisions:    make(map[ID]Decision, len(definitions)),
	}
	for _, definition := range definitions {
		requestedValue := requested[definition.ID]
		decision := Decision{
			ID:          definition.ID,
			Description: definition.Description,
			Tier:        tierName(definition.Tier),
			Requested:   requestedValue,
			Lifecycle:   string(definition.Lifecycle),
		}
		switch {
		case !requestedValue:
			decision.Reason = "not requested by runtime configuration"
		case disabled[definition.ID]:
			decision.Reason = "disabled by feature override"
		case !policy.Allows(definition, now):
			decision.Reason = "not included in the active product tier or license"
		default:
			decision.Allowed = true
			decision.Enabled = true
		}
		plan.Decisions[definition.ID] = decision
	}

	// Resolve dependencies after the first pass. Repeating the pass makes
	// transitive dependencies deterministic without relying on catalog order.
	for changed := true; changed; {
		changed = false
		for _, definition := range definitions {
			decision := plan.Decisions[definition.ID]
			if !decision.Enabled {
				continue
			}
			for _, dependency := range definition.Dependencies {
				if !plan.Decisions[dependency].Enabled {
					decision.Enabled = false
					decision.Reason = "dependency is disabled: " + string(dependency)
					changed = true
					break
				}
			}
			plan.Decisions[definition.ID] = decision
		}
	}
	return plan, nil
}

func validateIDs(ids []ID, known map[ID]Definition) error {
	seen := make(map[ID]bool, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(string(id)) == "" {
			return fmt.Errorf("feature override contains an empty id")
		}
		if _, ok := known[id]; !ok {
			return fmt.Errorf("unknown feature id %q", id)
		}
		if seen[id] {
			return fmt.Errorf("feature override contains duplicate id %q", id)
		}
		seen[id] = true
	}
	return nil
}

func tierName(tier Tier) string {
	if tier == Pro {
		return "pro"
	}
	return "community"
}

// Enabled reports only the effective state. Unknown IDs are always disabled.
func (p Plan) Enabled(id ID) bool {
	decision, ok := p.Decisions[id]
	return ok && decision.Enabled
}

// Decisions returns stable, human-friendly ordering for status endpoints.
func (p Plan) DecisionsList() []Decision {
	result := make([]Decision, 0, len(p.Decisions))
	for _, decision := range p.Decisions {
		result = append(result, decision)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

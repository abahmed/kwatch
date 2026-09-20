package statuswatch

import (
	"fmt"
	"strings"
)

// ConfigurePolicy installs status and graph-reference rules before the
// watcher starts. Policy is immutable for the lifetime of a monitor.
func (m *Monitor) ConfigurePolicy(
	conditionEntries []string,
	graphEntries []string,
) error {
	conditions, err := parseConditionRules(conditionEntries)
	if err != nil {
		return err
	}
	graphs, err := parseGraphReferenceRules(graphEntries)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started {
		return fmt.Errorf("status policy cannot change after processing starts")
	}
	m.conditionRules = conditions
	m.graphReferences = graphs
	return nil
}

func parseConditionRules(
	entries []string,
) (map[string]map[string]bool, error) {
	rules := make(map[string]map[string]bool)
	for _, entry := range entries {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" ||
			strings.TrimSpace(parts[1]) == "" {
			return nil, fmt.Errorf(
				"invalid condition rule %q: use ConditionType=Status",
				entry,
			)
		}
		typ := strings.TrimSpace(parts[0])
		status := strings.TrimSpace(parts[1])
		if rules[typ] == nil {
			rules[typ] = make(map[string]bool)
		}
		rules[typ][status] = true
	}
	return rules, nil
}

func parseGraphReferenceRules(
	entries []string,
) ([]graphReferenceRule, error) {
	rules := make([]graphReferenceRule, 0, len(entries))
	for _, entry := range entries {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" ||
			strings.TrimSpace(parts[1]) == "" {
			return nil, fmt.Errorf(
				"invalid graph reference %q: use path=kind", entry,
			)
		}
		path := make([]string, 0)
		for _, part := range strings.Split(strings.TrimSpace(parts[0]), ".") {
			if part != "" {
				path = append(path, part)
			}
		}
		kind := strings.ToLower(strings.TrimSpace(parts[1]))
		if len(path) == 0 || kind == "" {
			return nil, fmt.Errorf(
				"invalid graph reference %q: use path=kind", entry,
			)
		}
		rules = append(rules, graphReferenceRule{path: path, kind: kind})
	}
	return rules, nil
}

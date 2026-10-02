package delivery

import "strings"

// sanitizeFallbackCycles disables the first fallback that closes a cycle.
// Fallback delivery is intentionally single-hop, but rejecting the cycle in
// the generation keeps the configuration safe if traversal grows later.
// Only a provider on the cycle loses its fallback: with A->B, B->C, C->B,
// A keeps B and the walk from B disables B's fallback.
func sanitizeFallbackCycles(entries []providerEntry) []string {
	indexes := make(map[string]int, len(entries))
	for index, entry := range entries {
		indexes[entry.lookupName()] = index
	}

	var disabled []string
	for index := range entries {
		start := entries[index].lookupName()
		current := start
		seen := make(map[string]struct{})
		for current != "" {
			if _, exists := seen[current]; exists {
				if current == start {
					entries[index].fallbackName = ""
					disabled = append(disabled,
						entries[index].provider.Name())
				}
				break
			}
			seen[current] = struct{}{}
			next, exists := indexes[current]
			if !exists {
				break
			}
			current = strings.ToLower(entries[next].fallbackName)
		}
	}
	return disabled
}

func (entry providerEntry) lookupName() string {
	if entry.catalogName != "" {
		return strings.ToLower(entry.catalogName)
	}
	return strings.ToLower(entry.provider.Name())
}

package delivery

import "strings"

// sanitizeFallbackCycles disables the first fallback that closes a cycle.
// Fallback delivery is intentionally single-hop, but rejecting the cycle in
// the generation keeps the configuration safe if traversal grows later.
func sanitizeFallbackCycles(entries []providerEntry) []string {
	indexes := make(map[string]int, len(entries))
	for index, entry := range entries {
		indexes[strings.ToLower(entry.provider.Name())] = index
	}

	var disabled []string
	for index := range entries {
		current := strings.ToLower(entries[index].provider.Name())
		seen := make(map[string]struct{})
		for current != "" {
			if _, exists := seen[current]; exists {
				entries[index].fallbackName = ""
				disabled = append(disabled, entries[index].provider.Name())
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

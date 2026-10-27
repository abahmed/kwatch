package incident

import "unicode/utf8"

// maxFacts bounds how many captured lines an incident keeps.
const maxFacts = 25

// maxFactLength bounds how many bytes each captured line may occupy.
const maxFactLength = 512

// Detail is what the pipeline keeps on an incident for the dashboard
// detail page: the composer's headline, the runbook link and captured
// lines, all already redacted.
type Detail struct {
	Headline string
	DocsLink string
	Facts    []string
}

// StoreDetail records d on the incident. Empty headline or link keep the
// earlier value. Facts are appended once each, cut on a rune boundary and
// bounded to the most recent maxFacts. No-op for an unknown incident.
func (m *Manager) StoreDetail(id string, d Detail) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.incidents[id]
	if !ok {
		return
	}
	if d.Headline != "" {
		p.Headline = d.Headline
	}
	if d.DocsLink != "" {
		p.DocsLink = d.DocsLink
	}
	p.Facts = mergeFacts(p.Facts, d.Facts)
}

// mergeFacts appends the new lines that are not already held, keeping
// the most recent maxFacts.
func mergeFacts(held, fresh []string) []string {
	seen := make(map[string]bool, len(held)+len(fresh))
	for _, f := range held {
		seen[f] = true
	}
	for _, f := range fresh {
		f = truncateRunes(f, maxFactLength)
		if f == "" || seen[f] {
			continue
		}
		seen[f] = true
		held = append(held, f)
	}
	if len(held) > maxFacts {
		held = append([]string(nil), held[len(held)-maxFacts:]...)
	}
	return held
}

// truncateRunes cuts s to at most limit bytes without splitting a rune.
func truncateRunes(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

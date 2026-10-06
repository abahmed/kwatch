package compose

import "strings"

// outsideQuotes applies rewrite to the parts of text that are not inside
// double quotes. Quoted text is what a pod wrote; a rewrite of kwatch's own
// wording (kind spelling, tense, plural) must never change it.
func outsideQuotes(text string, rewrite func(string) string) string {
	if !strings.Contains(text, `"`) {
		return rewrite(text)
	}
	parts := strings.Split(text, `"`)
	for i := 0; i < len(parts); i += 2 {
		parts[i] = rewrite(parts[i])
	}
	return strings.Join(parts, `"`)
}

package compose

// minRepeatedQuote is the shortest quotation that counts as the same
// words said twice; a short one ("Ready") can recur by chance.
const minRepeatedQuote = 12

// repeatsQuote reports a sentence that quotes what an earlier sentence
// of the note already quotes, by value: the same line twice, whoever
// introduces it ("api fails with ..." and "It fails with ..."). It
// remembers the quotations of every sentence it is asked about.
func repeatsQuote(seen map[string]bool, text string) bool {
	repeated := false
	var fresh []string
	for _, seg := range segmentsOf(text) {
		if !seg.quoted || len(seg.text) < minRepeatedQuote {
			continue
		}
		if seen[seg.text] {
			repeated = true
		}
		fresh = append(fresh, seg.text)
	}
	if repeated {
		return true
	}
	for _, quote := range fresh {
		seen[quote] = true
	}
	return false
}

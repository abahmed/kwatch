package explain

import (
	"sort"
	"unicode"
	"unicode/utf8"
)

// dependencyScanMax bounds the text DependencyIn reads: the lines it
// is given are cut far shorter at ingest, and a longer one can only be
// a retry loop repeating itself.
const dependencyScanMax = 1 << 10

// failureSet holds the failure phrases of one text and what the gap
// checks between them and a name need of that text, computed once: the
// clause breaks and where words start. Each gap check is then two
// binary searches instead of a scan of the text between them.
type failureSet struct {
	spans [][]int
	// singles are the offsets of ";" and new lines; commas those of a
	// comma before a space: the clause breaks.
	singles, commas []int
	// starts are the offsets where a word (strings.Fields) begins.
	starts []int
}

func newFailureSet(lower string, spans [][]int) *failureSet {
	f := &failureSet{spans: spans}
	prevSpace := true
	for i, r := range lower {
		switch r {
		case ';', '\n':
			f.singles = append(f.singles, i)
		case ',':
			if i+1 < len(lower) && reSpace(lower[i+1]) {
				f.commas = append(f.commas, i)
			}
		}
		space := unicode.IsSpace(r)
		if !space && prevSpace {
			f.starts = append(f.starts, i)
		}
		prevSpace = space
	}
	return f
}

// reSpace is the \s of a regular expression: ASCII white space.
func reSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\f' || b == '\r'
}

// countIn counts the offsets of sorted in [from, to).
func countIn(sorted []int, from, to int) int {
	if from >= to {
		return 0
	}
	return sort.SearchInts(sorted, to) - sort.SearchInts(sorted, from)
}

// broken reports a clause break inside lower[from:to].
func (f *failureSet) broken(from, to int) bool {
	return countIn(f.singles, from, to) > 0 ||
		countIn(f.commas, from, to-1) > 0
}

// words counts the words of lower[from:to] as strings.Fields would:
// the word starts inside it, and the word it begins in the middle of.
func (f *failureSet) words(lower string, from, to int) int {
	if from >= to {
		return 0
	}
	n := countIn(f.starts, from, to)
	r, _ := utf8.DecodeRuneInString(lower[from:])
	at := sort.SearchInts(f.starts, from)
	if !unicode.IsSpace(r) &&
		(at == len(f.starts) || f.starts[at] != from) {
		n++
	}
	return n
}

// capText cuts text to at most n bytes, never inside a rune.
func capText(text string, n int) string {
	if len(text) <= n {
		return text
	}
	for n > 0 && !utf8.RuneStart(text[n]) {
		n--
	}
	return text[:n]
}

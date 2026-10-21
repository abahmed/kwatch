package format

import "sync"

const (
	// memoEntries bounds how many lines a Signer remembers; it is cleared
	// when full, which only costs a few recomputations.
	memoEntries = 4096
	// memoMaxText is the longest line remembered, so the memo's size is
	// bounded in bytes as well as in entries.
	memoMaxText = 512
)

// Signer is Signature with a bounded memo. Signature is a pure function
// and a tracker asks it for the same few lines on every update, so a
// remembered answer can never differ from a computed one. Each owner
// builds its own Signer: the memo is state, so it is not shared through a
// package variable. A Signer is safe for concurrent use.
type Signer struct {
	mu    sync.Mutex
	known map[string]string
}

// NewSigner builds a Signer with an empty memo.
func NewSigner() *Signer {
	return &Signer{known: map[string]string{}}
}

// Signature answers like the package function Signature, from the memo
// when the line was seen before. A line over memoMaxText is computed and
// not remembered.
func (s *Signer) Signature(text string) string {
	if len(text) > memoMaxText {
		return Signature(text)
	}
	s.mu.Lock()
	out, ok := s.known[text]
	s.mu.Unlock()
	if ok {
		return out
	}
	out = Signature(text)
	s.mu.Lock()
	if len(s.known) >= memoEntries {
		s.known = map[string]string{}
	}
	s.known[text] = out
	s.mu.Unlock()
	return out
}

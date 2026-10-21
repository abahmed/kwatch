package format

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestSignerAnswersLikeTheComputation(t *testing.T) {
	signer := NewSigner()
	lines := []string{
		"dial tcp 10.4.7.21:6379: connect: connection refused",
		"2026-09-29T10:00:01.123Z ERROR request failed",
		"worker api-7f9c6d8b5-x2k4p crashed",
		"",
		"   padded line 42  ",
	}
	for round := 0; round < 3; round++ {
		for _, line := range lines {
			if got, want := signer.Signature(line), Signature(line); got != want {
				t.Fatalf("round %d, %q: got %q, want %q",
					round, line, got, want)
			}
		}
	}
}

func TestSignerRepeatedLineDoesNotRecompute(t *testing.T) {
	signer := NewSigner()
	line := "dial tcp 10.4.7.21:6379: connect: connection refused"
	signer.Signature(line)
	allocs := testing.AllocsPerRun(100, func() { signer.Signature(line) })
	if allocs > 0 {
		t.Errorf("a remembered line allocated %v times per call", allocs)
	}
}

// Signature is a pure function: it keeps no state of its own. A caller
// that asks for the same line again many times owns a Signer.
func TestSignatureKeepsNoProcessWideMemo(t *testing.T) {
	line := "dial tcp 10.4.7.21:6379: connect: connection refused"
	Signature(line)
	allocs := testing.AllocsPerRun(100, func() { Signature(line) })
	if allocs == 0 {
		t.Error("Signature answered from a memo that outlives its caller")
	}
}

func TestSignerInstancesDoNotShareTheirMemo(t *testing.T) {
	first, second := NewSigner(), NewSigner()

	first.Signature("failed to reach 10.0.0.7:5432")

	if len(second.known) != 0 {
		t.Errorf("a second Signer holds %d lines it never saw",
			len(second.known))
	}
}

func TestSignerMemoStaysBounded(t *testing.T) {
	m := NewSigner()
	for i := 0; i < 3*memoEntries; i++ {
		m.Signature(fmt.Sprintf("error %d on node-%d", i, i))
		if len(m.known) > memoEntries {
			t.Fatalf("memo holds %d lines, bound is %d",
				len(m.known), memoEntries)
		}
	}
	long := strings.Repeat("x", memoMaxText+1)
	before := len(m.known)
	m.Signature(long)
	if len(m.known) != before {
		t.Error("a line over the size bound must not be remembered")
	}
}

func TestSignerIsSafeForConcurrentUse(t *testing.T) {
	m := NewSigner()
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 2000; i++ {
				line := fmt.Sprintf("failed to reach 10.0.0.%d:5432", i%300)
				if got, want := m.Signature(line), Signature(line); got != want {
					t.Errorf("got %q, want %q", got, want)
					return
				}
			}
		}()
	}
	wg.Wait()
}

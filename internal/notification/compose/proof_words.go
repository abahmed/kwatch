package compose

import (
	"strconv"

	"github.com/abahmed/kwatch/internal/rootcause"
)

// proofWords word one piece of evidence from its code and numbers. A
// proof without a code is written as its text; an empty result drops
// it.
func proofWords(p rootcause.Proof) string {
	if p.Code == "" {
		return p.Text
	}
	if write, ok := proofWriters[p.Code]; ok {
		return write(p)
	}
	return proofPhrases[p.Code]
}

// proofWriters word the proofs whose numbers or fields shape the
// sentence.
var proofWriters = map[rootcause.ProofCode]func(rootcause.Proof) string{
	rootcause.ProofDependentsFail: func(p rootcause.Proof) string {
		return dependentWords(p.Count, p.Total)
	},
	rootcause.ProofErrorsName: func(p rootcause.Proof) string {
		return errorWords(p.Count, p.Total)
	},
	rootcause.ProofCoOnset: func(p rootcause.Proof) string {
		switch {
		case p.Total == 0 && p.Count == 2:
			return "they started failing at the same time"
		case p.Total == 0:
			return "all started failing at the same time"
		case p.Count == 2:
			return "they started failing " + secondsWords(p.Total) +
				" apart"
		}
		return "all started failing within " + secondsWords(p.Total)
	},
	rootcause.ProofWorkloadsMeet: func(p rootcause.Proof) string {
		if p.Count == 2 {
			return "both failing apps use it"
		}
		return "all " + numberWord(p.Count) + " failing apps use it"
	},
	rootcause.ProofFillsDisks: func(p rootcause.Proof) string {
		return "its pods hold most of the disk in use on " +
			numberWord(p.Count) + " nodes"
	},
	rootcause.ProofExhaustsMemory: func(p rootcause.Proof) string {
		if p.Count == 1 {
			return "the kernel killed its process for memory there"
		}
		return "the kernel killed its process for memory on " +
			numberWord(p.Count) + " nodes"
	},
	rootcause.ProofChanged: func(p rootcause.Proof) string {
		return changedFieldsWords(p.Fields)
	},
	rootcause.ProofBaseline: func(p rootcause.Proof) string {
		return "it deviates " + strconv.Itoa(p.Count) +
			"% from its baseline"
	},
}

// proofPhrases word the proofs that need no numbers. A code missing
// here and in proofWriters is dropped: it only ever speaks against a
// cause, and writers show supporting proof only.
var proofPhrases = map[rootcause.ProofCode]string{
	// Timing alone convinces nobody reading the note.
	rootcause.ProofBeganBefore:     "",
	rootcause.ProofNothingUpstream: "nothing it depends on is failing",
	rootcause.ProofCreated:         "it was created shortly before",
	rootcause.ProofSiblingsHealthy: "replicas that do not depend on it " +
		"are healthy",
	rootcause.ProofRevertRecovered: "reverting its change brought the " +
		"apps that use it back",
	rootcause.ProofFailedAfterChange: "the apps that use it " +
		"failed soon after it changed",
	rootcause.ProofNewRevisionFails: "only the new revision fails; the " +
		"previous one stays healthy",
}

// dependentWords writes "the one app that uses it is failing", "every
// app that uses it is failing" or "two of the three apps that use it are
// failing".
func dependentWords(n, of int) string {
	switch {
	case of == 1 && n == 1:
		return "the one app that uses it is failing"
	case n == of:
		return "every app that uses it is failing"
	}
	return numberWord(n) + " of the " + numberWord(of) + " apps that " +
		"use it " + verb(n, "is", "are") + " failing"
}

// errorWords writes "the error mentions it", "every error mentions it"
// or "two of the four errors mention it".
func errorWords(n, of int) string {
	switch {
	case of == 1 && n == 1:
		return "the error mentions it"
	case n == of:
		return "every error mentions it"
	}
	return numberWord(n) + " of the " + numberWord(of) + " errors " +
		verb(n, "mentions", "mention") + " it"
}

// changedFieldsWords words a change by the fields it touched: "its
// selector changed shortly before".
func changedFieldsWords(paths []string) string {
	if len(paths) == 0 {
		return "it changed shortly before"
	}
	var fields []string
	for _, path := range paths {
		fields = append(fields, lastSegment(path))
	}
	return "its " + joinAnd(fields, maxNamed) + " changed shortly before"
}

// secondsWords is "4 seconds", or "1 second".
func secondsWords(n int) string {
	if n == 1 {
		return "1 second"
	}
	return strconv.Itoa(n) + " seconds"
}

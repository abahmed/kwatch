package detectors

import "github.com/abahmed/kwatch/internal/detection/reasons"

// Exit codes a container ends with. A shell reports 126 and 127 for a
// command it could not run, and 128+n for a process killed by signal n.
const (
	exitNotExecutable = 126
	exitNotFound      = 127
	exitSIGILL        = 128 + 4
	exitSIGABRT       = 128 + 6
	exitSIGBUS        = 128 + 7
	exitSIGFPE        = 128 + 8
	exitSIGKILL       = 128 + 9
	exitSIGSEGV       = 128 + 11
	exitSIGTERM       = 128 + 15
	// exitOutOfRange is what exit(-1) leaves, and what a runtime reports
	// when it cannot give a status.
	exitOutOfRange = 255
)

// exitMeaning names a container exit code with a well-known cause.
type exitMeaning struct {
	code    float64
	reason  string
	summary string
	// crashLoop reports whether a crash loop with this last exit code is
	// named after it. SIGKILL is not: liveness probe kills end with 137,
	// and the probe finding names that cause.
	crashLoop bool
}

// exitMeanings is the one decoder of exit codes. It lists the codes whose
// cause is unambiguous. 143 (SIGTERM) is not listed: it is how a
// container ends when Kubernetes stops it, handled by stoppedOnPurpose.
// Codes 1 and 2 are not listed either: they are the application's own
// error, so the finding keeps the generic wording with the code.
var exitMeanings = []exitMeaning{
	{exitNotExecutable, reasons.ExitNotExecutable,
		"cannot run its command: the file is not executable (exit " +
			"code 126)", true},
	{exitNotFound, reasons.ExitCommandNotFound,
		"cannot run its command: the command was not found (exit " +
			"code 127)", true},
	{exitSIGILL, reasons.ExitIllegalInstruction,
		"crashed with an illegal instruction (exit code 132)", true},
	{exitSIGABRT, reasons.ExitAborted,
		"aborted with SIGABRT (exit code 134)", true},
	{exitSIGBUS, reasons.ExitBusError,
		"crashed with a bus error (exit code 135)", true},
	{exitSIGFPE, reasons.ExitFloatingPoint,
		"crashed with an arithmetic error, SIGFPE (exit code 136)", true},
	{exitSIGKILL, reasons.ExitKilled,
		"was killed by SIGKILL (exit code 137) without running out " +
			"of memory", false},
	{exitSIGSEGV, reasons.ExitSegfault,
		"crashed with a segmentation fault (exit code 139)", true},
	// An out-of-range status says little, so it keeps the generic
	// reason and only explains the number.
	{exitOutOfRange, reasons.Error,
		"exited with code 255 (a status of -1 or out of range)", false},
}

// exitMeaningFor decodes an exit code; ok is false for codes without a
// well-known cause.
func exitMeaningFor(code float64) (exitMeaning, bool) {
	for _, meaning := range exitMeanings {
		if meaning.code == code {
			return meaning, true
		}
	}
	return exitMeaning{}, false
}

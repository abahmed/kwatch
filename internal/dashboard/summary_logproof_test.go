package dashboard

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func firstLogProof(lines ...string) string {
	d := IncidentDetail{CapturedLogLines: lines}
	got := logProof(&d)
	if len(got) == 0 {
		return ""
	}
	return got[0].Detail
}

// A wrapped .NET exception: the proof is the innermost cause, never
// the marker that ends the inner stack trace.
func TestLogProofSkipsTheDotNetTraceMarker(t *testing.T) {
	assert.Equal(t, "System.InvalidOperationException: Unable to "+
		"resolve service for type 'Foo.IBar'.", firstLogProof(
		"Unhandled exception. System.AggregateException: Some services "+
			"are not able to be constructed",
		"---> System.InvalidOperationException: Error while validating",
		"---> System.InvalidOperationException: Unable to resolve "+
			"service for type 'Foo.IBar'.",
		"--- End of inner exception stack trace ---"))
}

func TestLogProofReadsTheJavaRootCause(t *testing.T) {
	assert.Equal(t, "java.io.IOException: disk failure", firstLogProof(
		"Exception in thread \"main\" java.lang.IllegalStateException: "+
			"boot failed",
		"\tat com.example.Main.main(Main.java:10)",
		"Caused by: java.io.IOException: disk failure",
		"\tat com.example.Disk.read(Disk.java:4)",
		"... 3 more"))
}

func TestLogProofReadsThePythonException(t *testing.T) {
	assert.Equal(t, "ValueError: bad port", firstLogProof(
		"Traceback (most recent call last):",
		`  File "app.py", line 3, in <module>`,
		"ValueError: bad port"))
}

func TestLogProofReadsTheGoPanic(t *testing.T) {
	assert.Equal(t, "panic: nil map write", firstLogProof(
		"panic: nil map write",
		"goroutine 1 [running]:",
		"main.main()"))
}

func TestLogProofReadsTheNodeError(t *testing.T) {
	assert.Equal(t, "Error: connect ECONNREFUSED 10.0.0.1:5432",
		firstLogProof("Error: connect ECONNREFUSED 10.0.0.1:5432",
			"    at TCPConnectWrap.afterConnect (node:net:1:1)",
			"    at process.emit (node:events:1:1)"))
}

func TestLogProofKeepsTheNewestPlainErrorLine(t *testing.T) {
	assert.Equal(t, "FATAL could not open db", firstLogProof(
		"error: early", "listening on :80", "FATAL could not open db",
		"shutting down"))
	assert.Empty(t, firstLogProof("--- End of inner exception stack "+
		"trace ---", "   at Foo.Bar()"))
}

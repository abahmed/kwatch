package crashtest

import (
	"bufio"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	envMode  = "KWATCH_CRASH_MODE"
	envPath  = "KWATCH_CRASH_PATH"
	envStart = "KWATCH_CRASH_START"
	prefix   = "kwatch-crash "
)

// Child is the work of the child process. It writes forever: it calls
// ack(n) after write n is committed and say(text) at a step a test may
// wait for. Its first write is number start.
type Child func(path, mode string, start int, ack func(n int),
	say func(text string))

// Serve runs fn when this process is a crash child, and skips the test
// otherwise. A crash test has one test function that only calls Serve.
func Serve(t *testing.T, fn Child) {
	t.Helper()
	mode, path := os.Getenv(envMode), os.Getenv(envPath)
	if mode == "" {
		t.Skip("runs only as the child of a crash test")
	}
	start, _ := strconv.Atoi(os.Getenv(envStart))
	fn(path, mode, start,
		func(n int) { println(prefix + "ack " + strconv.Itoa(n)) },
		func(text string) { println(prefix + "say " + text) })
	os.Exit(3) // The child must never end by itself.
}

// Options says how to crash the child of the test function named Test.
type Options struct {
	Test, Mode, Path string
	// Start is the number of the child's first write.
	Start int
	// AfterAck kills once this write is acked. AfterSay kills once the
	// child said this text; it wins when set.
	AfterAck int
	AfterSay string
	// Delay is how long the child lives on after the trigger.
	Delay time.Duration
}

// Crash runs the child, kills it, and returns the last write it acked,
// or Start-1 when it acked none.
func Crash(t *testing.T, o Options) int {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^"+o.Test+"$")
	cmd.Env = append(os.Environ(), envMode+"="+o.Mode, envPath+"="+o.Path,
		envStart+"="+strconv.Itoa(o.Start))
	// The child writes its lines to stderr, where println goes.
	out, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	acked, killed := o.Start-1, false
	lines := bufio.NewScanner(out)
	for lines.Scan() {
		line, ok := strings.CutPrefix(lines.Text(), prefix)
		if !ok {
			continue
		}
		if n, ok := strings.CutPrefix(line, "ack "); ok {
			acked, _ = strconv.Atoi(n)
		}
		if !killed && triggered(o, line, acked) {
			killed = true
			time.Sleep(o.Delay)
			_ = cmd.Process.Kill()
		}
	}
	_ = cmd.Wait() // After the kill the pipe ends; acks seen late count.
	if !killed {
		t.Fatalf("the child ended before the crash point (%+v)", o)
	}
	return acked
}

func triggered(o Options, line string, acked int) bool {
	if o.AfterSay != "" {
		return line == "say "+o.AfterSay
	}
	return acked >= o.AfterAck
}

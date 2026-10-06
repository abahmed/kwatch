package audit

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoggerWritesEntriesAsJSONLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	logger := NewLogger(Config{Enabled: true, Output: path})
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	logger.Record(Entry{Timestamp: at, Action: ActionCreate, Incident: "p1"})
	logger.Record(Entry{Timestamp: at, Action: ActionResolved, Incident: "p1"})
	require.NoError(t, logger.Close())

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var actions []Action
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		var entry Entry
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &entry))
		require.Equal(t, "p1", entry.Incident)
		actions = append(actions, entry.Action)
	}
	require.Equal(t, []Action{ActionCreate, ActionResolved}, actions)
}

func TestDisabledLoggerDiscards(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	logger := NewLogger(Config{Enabled: false, Output: path})
	logger.Record(Entry{Incident: "p1"})
	require.NoError(t, logger.Close())
	_, err := os.Stat(path)
	require.True(t, os.IsNotExist(err))

	var nilLogger *Logger
	nilLogger.Record(Entry{})
	require.NoError(t, nilLogger.Close())
}

func TestLoggerFallsBackToStdoutOnBadPath(t *testing.T) {
	logger := NewLogger(Config{
		Enabled: true, Output: filepath.Join(t.TempDir(), "missing", "a"),
	})
	require.NotNil(t, logger.enc)
	require.Nil(t, logger.closer)
}

func TestRotatingFileRotatesWhenExceedingMaxBytes(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.log")
	maxBytes := int64(100)

	rf, err := openRotatingFile(filePath, maxBytes)
	require.NoError(t, err)
	defer rf.Close()

	firstWrite := []byte("First write content that is less than max\n")
	n, err := rf.Write(firstWrite)
	require.NoError(t, err)
	require.Equal(t, len(firstWrite), n)

	secondWrite := make([]byte, 80)
	for i := range secondWrite {
		secondWrite[i] = 'a'
	}
	n, err = rf.Write(secondWrite)
	require.NoError(t, err)
	require.Equal(t, len(secondWrite), n)

	rf.Close()

	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.True(t, int64(len(data)) < maxBytes)

	backupData, err := os.ReadFile(filePath + ".1")
	require.NoError(t, err)
	require.True(t, len(backupData) > 0)
}

func TestRotatingFilePreservesSizeAcrossReopen(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.log")
	maxBytes := int64(1000)

	rf, err := openRotatingFile(filePath, maxBytes)
	require.NoError(t, err)
	write1 := []byte("content\n")
	_, err = rf.Write(write1)
	require.NoError(t, err)
	rf.Close()

	rf, err = openRotatingFile(filePath, maxBytes)
	require.NoError(t, err)
	defer rf.Close()
	require.Equal(t, int64(len(write1)), rf.size)

	write2 := []byte("more\n")
	_, err = rf.Write(write2)
	require.NoError(t, err)

	rf.Close()
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, string(write1)+string(write2), string(data))
}

func TestRotatingFileKeepsWritingWhenRotationFails(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "audit.log")
	// A directory at the backup path makes the rename fail.
	require.NoError(t, os.Mkdir(filePath+".1", 0o700))

	rf, err := openRotatingFile(filePath, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rf.Close() })

	for i := 0; i < 3; i++ {
		n, err := rf.Write([]byte("0123456789\n"))
		require.NoError(t, err)
		require.Equal(t, 11, n)
	}
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, 33, len(data))
}

// When the rename works but the reopen does not, the renamed backup must
// not grow without bound, and the path is picked up again once it works.
func TestRotatingFileBoundsWritesWhenReopenFails(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "audit.log")
	rf, err := openRotatingFile(filePath, 20)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rf.Close() })
	clock := time.Unix(0, 0)
	rf.now = func() time.Time { return clock }
	healthy := rf.openFile
	rf.openFile = func(string) (*os.File, error) {
		return nil, errors.New("disk gone")
	}

	line := []byte("0123456789\n") // 11 bytes
	var dropped int
	for i := 0; i < 10; i++ {
		if _, err := rf.Write(line); errors.Is(err, errAuditDropped) {
			dropped++
		}
	}
	backup, err := os.ReadFile(filePath + ".1")
	require.NoError(t, err)
	require.LessOrEqual(t, len(backup), 2*20, "bounded by the cap")
	require.Greater(t, dropped, 0, "entries past the cap are dropped")

	rf.openFile = healthy
	clock = clock.Add(reopenBackoff)
	_, err = rf.Write(line)
	require.NoError(t, err)
	require.False(t, rf.detached)
	fresh, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, string(line), string(fresh))
}

// A rename that keeps failing is retried once per backoff, not once per
// entry, so it does not flood the log; entries are still written.
func TestRotatingFileRetriesAFailingRenameOncePerBackoff(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "audit.log")
	require.NoError(t, os.Mkdir(filePath+".1", 0o700))
	rf, err := openRotatingFile(filePath, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rf.Close() })
	clock := time.Unix(100, 0)
	rf.now = func() time.Time { return clock }
	line := []byte("0123456789\n")

	_, err = rf.Write(line)
	require.NoError(t, err)
	_, err = rf.Write(line) // size > max: rotation is tried and fails
	require.NoError(t, err)
	first := rf.rotateRetryAt
	require.Equal(t, clock.Add(reopenBackoff), first)

	clock = clock.Add(time.Second)
	_, err = rf.Write(line)
	require.NoError(t, err)
	require.Equal(t, first, rf.rotateRetryAt, "no retry inside the backoff")

	clock = clock.Add(reopenBackoff)
	_, err = rf.Write(line)
	require.NoError(t, err)
	require.NotEqual(t, first, rf.rotateRetryAt, "retried after it")
}

// When the new file opened, the rotation worked even if closing the old
// handle fails: the file must not be marked detached.
func TestRotatingFileIsNotDetachedByACloseError(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "audit.log")
	rf, err := openRotatingFile(filePath, 20)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rf.Close() })
	line := []byte("0123456789\n")
	_, err = rf.Write(line)
	require.NoError(t, err)
	old := rf.file
	_ = old.Close() // closing it again will fail

	_, err = rf.Write(line) // rotates: the old handle fails to close
	require.NoError(t, err)

	require.False(t, rf.detached)
	require.NotSame(t, old, rf.file)
	fresh, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, string(line), string(fresh))
}

package audit

import (
	"bufio"
	"bytes"
	"encoding/json"
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

	logger.Record(Entry{Timestamp: at, Action: ActionCreate, Problem: "p1"})
	logger.Record(Entry{Timestamp: at, Action: ActionResolved, Problem: "p1"})
	require.NoError(t, logger.Close())

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var actions []Action
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		var entry Entry
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &entry))
		require.Equal(t, "p1", entry.Problem)
		actions = append(actions, entry.Action)
	}
	require.Equal(t, []Action{ActionCreate, ActionResolved}, actions)
}

func TestDisabledLoggerDiscards(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	logger := NewLogger(Config{Enabled: false, Output: path})
	logger.Record(Entry{Problem: "p1"})
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

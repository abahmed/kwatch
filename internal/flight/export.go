package flight

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/replay"
	"github.com/abahmed/kwatch/internal/storage"
)

// ExportOptions say what to export.
type ExportOptions struct {
	// StatePath is the state file. It is copied, never opened in place.
	StatePath string
	// Since and Until bound the window; zero is open. Entries recorded
	// before Since that still describe the cluster are written first.
	Since, Until time.Time
	// Out is the file to write.
	Out string
}

// Summary says what an export wrote.
type Summary struct {
	// Entries counts every entry written, Baseline included.
	Entries int
	// Baseline counts the entries that restate the cluster as it was
	// when the window began.
	Baseline   int
	Start, End time.Time
}

// Export copies the state file next to Out, reads the copy read-only and
// writes the window as a log. A running kwatch holds the state file
// locked and writes to it, so the live file is not opened; a copy taken
// mid-write can fail to open, in which case run the export again.
func Export(opts ExportOptions) (Summary, error) {
	if _, err := os.Stat(opts.StatePath); err != nil {
		return Summary{}, fmt.Errorf("flight: state file: %w", err)
	}
	copyPath, err := copyFile(opts.StatePath, filepath.Dir(opts.Out))
	if err != nil {
		return Summary{}, err
	}
	defer func() { _ = os.Remove(copyPath) }()
	store, err := storage.Open(copyPath, storage.Options{
		Now: time.Now, DeferRepair: true,
	})
	if err != nil {
		return Summary{}, fmt.Errorf("flight: open copy: %w", err)
	}
	defer func() { _ = store.Close() }()
	tmp := opts.Out + ".partial"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return Summary{}, err
	}
	sum, err := ExportFrom(NewRing(store), opts.Since, opts.Until, file)
	if cerr := file.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return Summary{}, err
	}
	return sum, os.Rename(tmp, opts.Out)
}

// copyFile copies src to a new file in dir and returns its path.
func copyFile(src, dir string) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer func() { _ = in.Close() }()
	out, err := os.CreateTemp(dir, "state.copy*")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(out.Name())
		return "", err
	}
	return out.Name(), out.Close()
}

// ExportFrom writes the window of ring as a replay log to w. With a
// Since, the log starts at it, and the latest state of every object seen
// before it comes first, so a replay starts from the cluster as it was.
func ExportFrom(
	ring Ring, since, until time.Time, w io.Writer,
) (Summary, error) {
	start := since
	var baseline []replay.Entry
	if since.IsZero() {
		first, ok, err := firstEntry(ring, until)
		if err != nil || !ok {
			return Summary{}, errOrEmpty(err)
		}
		start = first
	} else {
		var err error
		if baseline, err = baselineBefore(ring, since); err != nil {
			return Summary{}, err
		}
	}
	enc, err := replay.NewEncoder(w, start)
	if err != nil {
		return Summary{}, err
	}
	sum := Summary{Start: start, Baseline: len(baseline)}
	write := func(e replay.Entry) error {
		sum.Entries++
		sum.End = e.At
		return enc.Entry(e)
	}
	for _, e := range baseline {
		e.At = start
		if err := write(e); err != nil {
			return Summary{}, err
		}
	}
	inWindow := sum.Entries
	if err := ring.Walk(since, until, write); err != nil {
		return Summary{}, err
	}
	if sum.Entries == inWindow && len(baseline) == 0 {
		return Summary{}, ErrNoRecording
	}
	return sum, nil
}

func errOrEmpty(err error) error {
	if err != nil {
		return err
	}
	return ErrNoRecording
}

// firstEntry is the time of the first recorded entry before until.
func firstEntry(ring Ring, until time.Time) (time.Time, bool, error) {
	var first time.Time
	err := ring.Walk(time.Time{}, until, func(e replay.Entry) error {
		first = e.At
		return errStop
	})
	return first, !first.IsZero(), err
}

// baselineBefore returns, for every object recorded before since and not
// deleted since, the latest Observed and Related entries about it, oldest
// first.
func baselineBefore(ring Ring, since time.Time) ([]replay.Entry, error) {
	latest := map[string]replay.Entry{}
	err := ring.Walk(time.Time{}, since, func(e replay.Entry) error {
		o := e.Observation
		switch o.Kind {
		case inventory.Observed, inventory.Related:
			latest[stateKey(o)] = e
		case inventory.Gone:
			forget(latest, o.Entity)
		}
		return nil
	})
	out := make([]replay.Entry, 0, len(latest))
	for _, e := range latest {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if !a.At.Equal(b.At) {
			return a.At.Before(b.At)
		}
		return stateKey(a.Observation) < stateKey(b.Observation)
	})
	return out, err
}

func stateKey(o inventory.Observation) string {
	return o.Entity.String() + "|" + o.Source + "|" + string(o.Relation)
}

// forget drops every remembered state of id.
func forget(latest map[string]replay.Entry, id inventory.EntityID) {
	for key, e := range latest {
		if e.Observation.Entity == id {
			delete(latest, key)
		}
	}
}

// ParseWindow turns --since and --until into times. Each is either a
// duration back from now ("2h") or an RFC 3339 time. An empty until is
// open; an empty since is open too.
func ParseWindow(
	since, until string, now time.Time,
) (time.Time, time.Time, error) {
	from, err := parseBound(since, now)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("since: %w", err)
	}
	to, err := parseBound(until, now)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("until: %w", err)
	}
	if !from.IsZero() && !to.IsZero() && !to.After(from) {
		return time.Time{}, time.Time{},
			fmt.Errorf("until %s is not after since %s", to, from)
	}
	return from, to, nil
}

func parseBound(text string, now time.Time) (time.Time, error) {
	if text == "" {
		return time.Time{}, nil
	}
	if d, err := time.ParseDuration(text); err == nil && d >= 0 {
		return now.Add(-d), nil
	}
	at, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return time.Time{}, fmt.Errorf(
			"%q is neither a duration like 2h nor an RFC 3339 time", text)
	}
	return at, nil
}

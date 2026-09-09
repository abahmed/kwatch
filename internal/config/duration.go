package config

import (
	"encoding/json"
	"fmt"
	"time"
)

// Seconds and Minutes are integer durations that also accept a duration
// string.
//
// kwatch expressed one concept in four units: correlation.window in minutes,
// resolveHoldDown in seconds, smartGrouping.windowSeconds in seconds,
// pvcMonitor.interval in minutes. Every one of them is a bare integer in
// YAML, so nothing in the file says which is which and "window: 30" reads as
// half an hour to anyone who has just read "resolveHoldDown: 300". These
// types keep the integer meaning exactly as it was -- an upgrade changes
// nothing -- while also accepting "30s", "10m", "2h" for people who would
// rather say what they mean.
type Seconds int

// Minutes is Seconds' counterpart for the fields whose integer form counts
// minutes.
type Minutes int

// UnmarshalYAML accepts an integer (seconds) or a duration string.
func (s *Seconds) UnmarshalYAML(unmarshal func(interface{}) error) error {
	value, err := unmarshalDuration(unmarshal, time.Second)
	if err != nil {
		return err
	}
	*s = Seconds(value / time.Second)
	return nil
}

// UnmarshalYAML accepts an integer (minutes) or a duration string.
func (m *Minutes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	value, err := unmarshalDuration(unmarshal, time.Minute)
	if err != nil {
		return err
	}
	*m = Minutes(value / time.Minute)
	return nil
}

// MarshalJSON keeps the generated catalogs and any JSON round-trip on the
// integer form these fields have always had.
func (s Seconds) MarshalJSON() ([]byte, error) { return json.Marshal(int(s)) }

func (m Minutes) MarshalJSON() ([]byte, error) { return json.Marshal(int(m)) }

// Duration renders the configured value.
func (s Seconds) Duration() time.Duration {
	return time.Duration(s) * time.Second
}

func (m Minutes) Duration() time.Duration {
	return time.Duration(m) * time.Minute
}

// unmarshalDuration reads either a plain number, in units of unit, or a Go
// duration string. A string that is not a duration is an error rather than a
// silent zero, which would disable the setting it configures.
func unmarshalDuration(
	unmarshal func(interface{}) error,
	unit time.Duration,
) (time.Duration, error) {
	var number int
	if err := unmarshal(&number); err == nil {
		return time.Duration(number) * unit, nil
	}
	var text string
	if err := unmarshal(&text); err != nil {
		return 0, fmt.Errorf(
			"expected a number of %s or a duration string: %w", unit, err,
		)
	}
	parsed, err := time.ParseDuration(text)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", text, err)
	}
	if parsed%unit != 0 {
		return 0, fmt.Errorf(
			"duration %q is not a whole number of %s", text, unit,
		)
	}
	return parsed, nil
}

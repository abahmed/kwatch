package config

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Bounds of dashboard.sessionLifetime and dashboard.signInHelp.
const (
	// MinSessionLifetime is the shortest sign-in: shorter would sign
	// people out while they read an incident.
	MinSessionLifetime = 15 * time.Minute
	// MaxSessionLifetime is the longest sign-in, seven days: a stolen
	// cookie must not work for weeks.
	MaxSessionLifetime = 7 * 24 * time.Hour
	// MaxSignInHelp bounds the sign-in help text, in characters.
	MaxSignInHelp = 500
)

// Lifetime is how long a dashboard sign-in lasts: the parsed
// sessionLifetime, or 0 for the dashboard's default when it is empty or
// not valid (validation reports that).
func (d Dashboard) Lifetime() time.Duration {
	life, err := parseSessionLifetime(d.SessionLifetime)
	if err != nil {
		return 0
	}
	return life
}

func parseSessionLifetime(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	life, err := time.ParseDuration(raw)
	if err != nil || life < MinSessionLifetime ||
		life > MaxSessionLifetime {
		return 0, fmt.Errorf("dashboard.sessionLifetime %q must be a "+
			"duration between %s and %s", raw, MinSessionLifetime,
			MaxSessionLifetime)
	}
	return life, nil
}

// validateDashboardSession checks the session lifetime and the sign-in
// help text: plain text of at most MaxSignInHelp characters, with no
// control characters but line breaks.
func validateDashboardSession(d Dashboard) []error {
	var errs []error
	if _, err := parseSessionLifetime(d.SessionLifetime); err != nil {
		errs = append(errs, err)
	}
	if n := utf8.RuneCountInString(d.SignInHelp); n > MaxSignInHelp {
		errs = append(errs, fmt.Errorf("dashboard.signInHelp has %d "+
			"characters; at most %d", n, MaxSignInHelp))
	}
	if !utf8.ValidString(d.SignInHelp) ||
		strings.IndexFunc(d.SignInHelp, controlRune) >= 0 {
		errs = append(errs, errors.New("dashboard.signInHelp must be "+
			"plain text without control characters"))
	}
	return errs
}

func controlRune(r rune) bool {
	return unicode.IsControl(r) && r != '\n'
}

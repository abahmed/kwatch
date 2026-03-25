package common

import (
	"regexp"
	"slices"
	"strings"
)

// Contains checks if value is in list
func Contains(value string, list []string) bool {
	return slices.Contains(list, value)
}

// Regex checks if value matches any pattern
func Regex(value string, patterns []*regexp.Regexp) bool {
	for _, p := range patterns {
		if p.MatchString(value) {
			return true
		}
	}
	return false
}

// Substring checks if value contains any substring
func Substring(value string, substrings []string) bool {
	for _, s := range substrings {
		if strings.Contains(value, s) {
			return true
		}
	}
	return false
}

// FilterByList checks string against allowed/forbidden lists
func FilterByList(value string, allowed, forbidden []string) bool {
	if len(allowed) > 0 && !Contains(value, allowed) {
		return true
	}
	if len(forbidden) > 0 && Contains(value, forbidden) {
		return true
	}
	return false
}

package message

import "strings"

func meaningfulRecoveryDetail(detail string) bool {
	detail = strings.TrimSpace(detail)
	switch detail {
	case "", "the condition recovered",
		"recovery was verified by the lifecycle check":
		return false
	default:
		return true
	}
}

package filter

import (
	"strings"

	"github.com/abahmed/kwatch/internal/config"
)

// MatchesContainerName reports whether a container name is silenced.
func MatchesContainerName(
	index config.SuppressionIndex,
	name string,
) bool {
	for _, candidate := range index.ContainerNames {
		if candidate == name {
			return true
		}
	}
	return false
}

// MatchesPodName reports whether a pod name matches a silenced pattern.
func MatchesPodName(index config.SuppressionIndex, name string) bool {
	for _, pattern := range index.PodNamePatterns {
		if pattern.MatchString(name) {
			return true
		}
	}
	return false
}

// MatchesLog reports whether log output matches a silenced pattern.
func MatchesLog(index config.SuppressionIndex, logs string) bool {
	for _, pattern := range index.LogPatterns {
		if pattern.MatchString(logs) {
			return true
		}
	}
	return false
}

// MatchesContainerMessage reports whether a container message is silenced.
func MatchesContainerMessage(
	index config.SuppressionIndex,
	message string,
) bool {
	return containsAny(message, index.ContainerMessages)
}

// MatchesEventMessage reports whether an event message is silenced.
func MatchesEventMessage(
	index config.SuppressionIndex,
	message string,
) bool {
	return message != "" && containsAny(message, index.EventMessages)
}

// MatchesNodeReason reports whether a node reason is silenced.
func MatchesNodeReason(index config.SuppressionIndex, reason string) bool {
	return containsExact(reason, index.NodeReasons)
}

// MatchesNodeMessage reports whether a node message is silenced.
func MatchesNodeMessage(index config.SuppressionIndex, message string) bool {
	return containsAny(message, index.NodeMessages)
}

func containsExact(value string, candidates []string) bool {
	for _, candidate := range candidates {
		if value == candidate {
			return true
		}
	}
	return false
}

func containsAny(value string, candidates []string) bool {
	for _, candidate := range candidates {
		if candidate != "" && strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

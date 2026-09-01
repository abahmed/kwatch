package util

import (
	"crypto/rand"
	"math/big"

	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"
)

const randomStringAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLM" +
	"NOPQRSTUVWXYZ0123456789"

// OrDefault returns s if non-empty, otherwise returns def.
func OrDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// RandomString returns a cryptographically random alphanumeric string.
func RandomString(length int) string {
	if length <= 0 {
		return ""
	}
	result := make([]byte, length)
	limit := big.NewInt(int64(len(randomStringAlphabet)))
	for i := range result {
		index, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return ""
		}
		result[i] = randomStringAlphabet[index.Int64()]
	}
	return string(result)
}

// Chunks splits s into UTF-8-safe slices whose byte length is at most
// chunkSize where possible. Provider limits are byte limits, not rune limits.
func Chunks(s string, chunkSize int) []string {
	if chunkSize <= 0 || chunkSize >= len(s) {
		return []string{s}
	}

	chunks := make([]string, 0, (len(s)-1)/chunkSize+1)
	currentStart := 0

	for i := range s {
		if i > currentStart && i-currentStart >= chunkSize {
			chunks = append(chunks, s[currentStart:i])
			currentStart = i
		}
	}

	chunks = append(chunks, s[currentStart:])
	return chunks
}

// RenderIncident renders an incident using the Report model and the given
// renderer, returning the formatted text. Returns empty string for skip
// actions.
func RenderIncident(
	inc *model.Incident,
	action model.IncidentAction,
	renderer message.Renderer,
	clusterName string,
) string {
	return RenderIncidentWithInsight(inc, action, nil, renderer, clusterName)
}

// RenderIncidentWithInsight is RenderIncident with the insight engine's
// diagnosis included. Every provider that renders through the Report model
// gets cause, impact and recent changes from this one place — before it, ten
// providers hard-coded nil here and silently dropped the diagnosis.
func RenderIncidentWithInsight(
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
	renderer message.Renderer,
	clusterName string,
) string {
	if action == model.ActionSkip {
		return ""
	}
	report := message.NewReportBuilder(clusterName).Build(inc, action, ins)
	return message.RenderAction(renderer, report)
}

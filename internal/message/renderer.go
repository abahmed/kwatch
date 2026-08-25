package message

// Renderer converts a Report into a formatted string for a specific
// communication platform. Each platform implements its own Renderer
// to produce platform-native formatting (Slack mrkdwn, Discord markdown,
// plain text, etc.).
type Renderer interface {
	RenderCreate(r *Report) string
	RenderUpdate(r *Report) string
	RenderResolved(r *Report) string
}

// RenderAction dispatches to the correct Renderer method based on action.
func RenderAction(renderer Renderer, r *Report) string {
	switch r.Action {
	case "create":
		return renderer.RenderCreate(r)
	case "update":
		return renderer.RenderUpdate(r)
	case "resolved":
		return renderer.RenderResolved(r)
	default:
		return ""
	}
}

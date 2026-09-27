package telegram

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/event"
)

func TestTelegramRenderedMessageUsesPlainText(t *testing.T) {
	c := NewTelegram(
		map[string]interface{}{"token": "test", "chatId": "test"},
		testAppConfig(), testDeps,
	)
	body := c.buildRequestBodyTelegram(
		new(event.Event), "chat", "pod my_app crashed: *oops* [x]",
	)
	assert.NotContains(t, body, "parse_mode")
	assert.Contains(t, body, "my_app crashed: *oops* [x]")
}

func TestTelegramEventMarkdownEscapesEventData(t *testing.T) {
	c := NewTelegram(
		map[string]interface{}{"token": "test", "chatId": "test"},
		testAppConfig(), testDeps,
	)
	body := c.buildRequestBodyTelegram(&event.Event{
		PodName: "my_app", Reason: "Error", IncludeLogs: true,
		Logs: "panic: *bad* `x` [y]",
	}, "chat", "")
	assert.Contains(t, body, `"parse_mode":"MARKDOWN"`)
	assert.Contains(t, body, `my\\_app`)
	assert.Contains(t, body, `panic: \\*bad\\* \\`+"`"+`x\\`+"`"+` \\[y]`)
}

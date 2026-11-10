package telegram

import (
	"context"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// A plain message over Telegram's 4096 character limit is cut instead of
// being rejected for good.
func TestTelegramCutsOverlongMessage(t *testing.T) {
	c, rec := recorderTelegram(t)
	if err := c.SendMessage(context.Background(),
		strings.Repeat("x", 20000)); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	providertest.AssertWithin(t, "text",
		rec.Last(t).JSON(t)["text"].(string), telegramTextLimit)
}

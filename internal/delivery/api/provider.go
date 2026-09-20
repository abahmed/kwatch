// Package api contains the small contracts shared by delivery and provider
// catalog packages. It intentionally has no delivery implementation logic.
package api

import (
	"context"

	"github.com/abahmed/kwatch/internal/event"
)

// Provider is the canonical, cancellation-aware provider contract.
type Provider interface {
	Name() string
	SendEvent(context.Context, *event.Event) error
	SendMessage(context.Context, string) error
}

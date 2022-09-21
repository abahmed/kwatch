package alertmanager

import (
	"github.com/abahmed/kwatch/event"
)

// Provider interface
type Provider interface {
	Name() string
	SendEvent(*event.Event) error
	SendMessage(string) error
}

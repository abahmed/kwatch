package provider

import "github.com/abahmed/kwatch/event"

const (
	footer       = "<https://github.com/abahmed/kwatch|kwatch>"
	defaultTitle = ":red_circle: kwatch detected a crash in pod"
	defaultText  = "There is an issue with container in a pod!"
)

// Provider interface
type Provider interface {
	Name() string
	SendEvent(*event.Event) error
	SendMessage(string) error
}

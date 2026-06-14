package filter

import "strings"

type ContainerMessageFilter struct {
	Messages []string
}

func (f ContainerMessageFilter) Detect(ctx *Context) Status {
	if len(f.Messages) == 0 || ctx.Container == nil || ctx.Container.Container == nil {
		return StatusAlert
	}

	cs := ctx.Container.Container
	msg := ""
	if cs.State.Waiting != nil {
		msg = cs.State.Waiting.Message
	} else if cs.State.Terminated != nil {
		msg = cs.State.Terminated.Message
	}

	if msg == "" {
		return StatusAlert
	}

	for _, pattern := range f.Messages {
		if strings.Contains(msg, pattern) {
			return StatusSkip
		}
	}

	return StatusAlert
}

func (f ContainerMessageFilter) Execute(ctx *Context) bool {
	return f.Detect(ctx) == StatusSkip
}

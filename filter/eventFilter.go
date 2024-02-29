package filter

type EventFilter struct{}

func (f EventFilter) Execute(ctx *Context) bool {
	if ctx.EvType == "DELETED" {
		ctx.Memory.DelPod(ctx.Pod.Namespace, ctx.Pod.Name)
		return true
	}

	return false
}

package filter

import (
	"github.com/sirupsen/logrus"
)

type PodNameFilter struct{}

func (f PodNameFilter) Execute(ctx *Context) bool {
	for _, pattern := range ctx.Config.IgnorePodNamePatterns {
		if pattern.MatchString(ctx.Pod.Name) {
			logrus.Infof(
				"skipping pod %s as it is in the ignore pod name list",
				ctx.Pod.Name)
			return true
		}
	}

	return false
}

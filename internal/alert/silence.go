package alert

import (
	"regexp"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
)

type silenceMatcher struct {
	namespaces     []string
	reasons        []string
	podPattern     []*regexp.Regexp
	containerNames []string
	logPatterns    []*regexp.Regexp
	containerMsgs  []string
	nodeReasons    []string
	nodeMessages   []string
}

// Provider interface

func (a *AlertManager) SetSilences(rules []config.SilenceRule) {
	built := make([]silenceMatcher, 0, len(rules))
	for _, sr := range rules {
		sm := silenceMatcher{
			namespaces:     sr.Namespaces,
			reasons:        sr.Reasons,
			containerNames: sr.ContainerNames,
			containerMsgs:  sr.ContainerMessages,
			nodeReasons:    sr.NodeReasons,
			nodeMessages:   sr.NodeMessages,
		}
		for _, p := range sr.PodNamePatterns {
			if re, err := regexp.Compile(p); err == nil {
				sm.podPattern = append(sm.podPattern, re)
			} else {
				klog.ErrorS(err, "invalid silence pod name pattern", "pattern", p)
			}
		}
		for _, p := range sr.LogPatterns {
			if re, err := regexp.Compile(p); err == nil {
				sm.logPatterns = append(sm.logPatterns, re)
			} else {
				klog.ErrorS(err, "invalid silence log pattern", "pattern", p)
			}
		}
		built = append(built, sm)
	}
	a.cfgMu.Lock()
	a.silences = built
	a.cfgMu.Unlock()
}

func (a *AlertManager) isSilenced(inc *model.Incident) bool {
	a.cfgMu.RLock()
	silences := a.silences
	a.cfgMu.RUnlock()
	for _, sm := range silences {
		if matchesSilence(sm, inc) {
			return true
		}
	}
	return false
}
func matchesSilence(sm silenceMatcher, inc *model.Incident) bool {
	if len(sm.namespaces) > 0 && !anyEq(sm.namespaces, inc.Namespace) {
		return false
	}
	if len(sm.reasons) > 0 && !anyEq(sm.reasons, inc.Reason) {
		return false
	}
	if len(sm.podPattern) > 0 && !anyRegex(sm.podPattern, inc.Name) {
		return false
	}
	if len(sm.containerNames) > 0 && !anyContainer(sm.containerNames, inc) {
		return false
	}
	if len(sm.logPatterns) > 0 && !anyRegex(sm.logPatterns, inc.Logs) {
		return false
	}
	if len(sm.containerMsgs) > 0 {
		if inc.LastContainerState == nil || !anyContains(sm.containerMsgs, inc.LastContainerState.Msg) {
			return false
		}
	}
	if len(sm.nodeReasons) > 0 && !anyEq(sm.nodeReasons, inc.Reason) {
		return false
	}
	if len(sm.nodeMessages) > 0 && !anyContains(sm.nodeMessages, inc.Hint) {
		return false
	}
	return true
}

func anyEq(items []string, want string) bool {
	for _, it := range items {
		if it == want {
			return true
		}
	}
	return false
}

func anyRegex(items []*regexp.Regexp, s string) bool {
	for _, re := range items {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

func anyContains(items []string, s string) bool {
	for _, it := range items {
		if strings.Contains(s, it) {
			return true
		}
	}
	return false
}

func anyContainer(items []string, inc *model.Incident) bool {
	for _, cn := range items {
		if cn == inc.ContainerName || inc.Containers[cn] {
			return true
		}
	}
	return false
}

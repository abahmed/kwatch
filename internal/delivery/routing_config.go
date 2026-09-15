package delivery

import (
	"strings"
	"text/template"

	"k8s.io/klog/v2"
)

func compileTemplates(
	rawTemplates map[string]string,
) map[string]*template.Template {
	if len(rawTemplates) == 0 {
		return nil
	}
	templates := make(map[string]*template.Template, len(rawTemplates))
	for reason, body := range rawTemplates {
		tmpl, err := template.New(reason).
			Option("missingkey=zero").Parse(body)
		if err != nil {
			klog.ErrorS(
				err,
				"invalid provider template, skipping",
				"reason",
				reason,
			)
			continue
		}
		templates[strings.ToLower(reason)] = tmpl
	}
	return templates
}

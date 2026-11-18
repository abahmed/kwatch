package config

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/format"
)

// gitOpsLinkTools are the tools app.gitOpsLinks may name: the ones
// kwatch recognizes on an object (kube.GitOpsApp).
var gitOpsLinkTools = map[string]bool{
	"argocd": true, "flux": true, "helm": true,
}

// validateGitOpsLinks requires every GitOps link to name a known tool
// and to be an absolute http or https address with {app} in it,
// because it is placed in alert messages and the dashboard as a link.
func validateGitOpsLinks(cfg *Config) []error {
	var errs []error
	for _, tool := range sortedStringKeys(cfg.App.GitOpsLinks) {
		if !gitOpsLinkTools[tool] {
			errs = append(errs, fmt.Errorf(
				"app.gitOpsLinks[%q] names no known tool: use argocd, "+
					"flux or helm", tool))
			continue
		}
		if !format.ValidLinkTemplate(cfg.App.GitOpsLinks[tool]) {
			errs = append(errs, fmt.Errorf(
				"app.gitOpsLinks[%q] must be an absolute http or https "+
					"URL containing %s", tool, format.AppPlaceholder))
		}
	}
	return errs
}

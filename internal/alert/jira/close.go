package jira

import (
	"context"
	"encoding/json"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// Close comments the recovery, then moves the issue through the
// configured closeTransition, if any. Workflows differ per
// project, so a missing or refused transition is logged and the issue is
// left as it is: the comment still tells the story, and retrying a
// refusal would never succeed.
func (g *Jira) Close(ctx context.Context, id, body string) error {
	if err := g.Comment(ctx, id, body); err != nil {
		return err
	}
	if g.closeTransition == "" {
		return nil
	}
	return g.transition(ctx, id, g.closeTransition)
}

// ClosesIssues is checked by the issue map: without a closeTransition a
// resolve only comments, so the issue stays open.
func (g *Jira) ClosesIssues() bool { return g.closeTransition != "" }

// CanReopen is checked by the issue map: an issue can be reopened only
// when a closing transition and a reopening transition are both set.
func (g *Jira) CanReopen() bool {
	return g.closeTransition != "" && g.reopenTransition != ""
}

// Reopen implements issues.Reopener: it applies the configured
// reopenTransition so the issue is open again before the comment.
func (g *Jira) Reopen(ctx context.Context, id string) error {
	if g.reopenTransition == "" {
		return nil
	}
	return g.transition(ctx, id, g.reopenTransition)
}

// transition finds the named transition among those the issue offers
// now and applies it.
func (g *Jira) transition(ctx context.Context, id, name string) error {
	url := g.url + "/" + id + "/transitions"
	answer, err := g.call(ctx, "GET", url, nil)
	if err != nil {
		return g.transitionFailure(id, "list transitions", err)
	}
	var offered struct {
		Transitions []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			To   struct {
				Name string `json:"name"`
			} `json:"to"`
		} `json:"transitions"`
	}
	if err := json.Unmarshal(answer, &offered); err != nil {
		return nil
	}
	for _, t := range offered.Transitions {
		if strings.EqualFold(t.Name, name) ||
			strings.EqualFold(t.To.Name, name) {
			_, err := g.call(ctx, "POST", url, map[string]interface{}{
				"transition": map[string]string{"id": t.ID},
			})
			return g.transitionFailure(id, "apply transition", err)
		}
	}
	klog.InfoS("jira issue has no such transition; left as is",
		"component", "delivery", "provider", g.Name(), "issue", id,
		"transition", name)
	return nil
}

// transitionFailure keeps a transient failure (so delivery retries) and
// logs a permanent one, which a retry cannot fix.
func (g *Jira) transitionFailure(id, step string, err error) error {
	if err == nil || !transport.IsPermanent(err) {
		return err
	}
	klog.InfoS("jira could not close the issue; left as is",
		"component", "delivery", "provider", g.Name(), "issue", id,
		"step", step, "error", err)
	return nil
}

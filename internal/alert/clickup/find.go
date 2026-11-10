package clickup

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
)

// findPages bounds the search: ClickUp has no text search for the tasks
// of a list, so the newest open tasks are listed and checked for the
// marker. A task lost to a timeout is always among the newest.
const findPages = 5

// Find implements issues.Finder: it lists the newest open tasks of the
// list and returns the one whose description carries marker.
func (g *Clickup) Find(ctx context.Context, marker string) (string, error) {
	for page := 0; page < findPages; page++ {
		id, last, err := g.findOnPage(ctx, marker, page)
		if err != nil || id != "" || last {
			return id, err
		}
	}
	return "", nil
}

func (g *Clickup) findOnPage(
	ctx context.Context, marker string, page int,
) (id string, last bool, err error) {
	response, err := g.call(ctx, "GET", g.url+
		"?include_markdown_description=true&order_by=created"+
		"&reverse=true&page="+strconv.Itoa(page), nil)
	if err != nil {
		return "", false, err
	}
	var found struct {
		Tasks []struct {
			ID                  string `json:"id"`
			Description         string `json:"description"`
			MarkdownDescription string `json:"markdown_description"`
		} `json:"tasks"`
		LastPage bool `json:"last_page"`
	}
	if json.Unmarshal(response, &found) != nil {
		return "", true, nil
	}
	for _, task := range found.Tasks {
		if strings.Contains(task.Description, marker) ||
			strings.Contains(task.MarkdownDescription, marker) {
			return task.ID, false, nil
		}
	}
	return "", found.LastPage || len(found.Tasks) == 0, nil
}

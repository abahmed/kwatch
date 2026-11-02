package dashboard

import (
	"errors"
	"net/url"
	"slices"
	"strconv"
	"time"
)

// List paging: the page sizes /api/incidents accepts, the default one,
// and the largest page number (far past any real list: kwatch keeps
// incidents a week).
var pageSizes = []int{25, 50, 100}

const (
	defaultPageSize = 50
	maxPage         = 100_000
)

// listRequest is a parsed /api/incidents query: the filter, the order
// and the page (1-based).
type listRequest struct {
	filter   filter
	sort     string
	desc     bool
	page     int
	pageSize int
}

// parseListRequest reads the query. A value it doesn't know is an
// error with a plain message for a 400.
func parseListRequest(q url.Values, now time.Time) (listRequest, error) {
	f, err := parseFilter(q, now)
	if err != nil {
		return listRequest{}, err
	}
	req := listRequest{filter: f}
	if req.sort, req.desc, err = parseSort(q.Get("sort"),
		q.Get("dir")); err != nil {
		return req, err
	}
	if req.page, err = parsePage(q.Get("page")); err != nil {
		return req, err
	}
	req.pageSize, err = parsePageSize(q.Get("pageSize"))
	return req, err
}

// parseSort reads sort (a field of sortFields) and dir (asc or desc;
// empty is the field's own direction).
func parseSort(field, dir string) (string, bool, error) {
	spec, ok := sortFields[field]
	if field != "" && !ok {
		return "", false, errors.New("invalid sort")
	}
	switch dir {
	case "":
		return field, spec.descends, nil
	case "asc", "desc":
		return field, dir == "desc", nil
	}
	return "", false, errors.New("invalid dir")
}

// parsePage reads ?page=: empty is the first page.
func parsePage(raw string) (int, error) {
	if raw == "" {
		return 1, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > maxPage {
		return 0, errors.New("invalid page")
	}
	return n, nil
}

// parsePageSize reads ?pageSize=: empty is the default, anything but
// one of pageSizes is invalid.
func parsePageSize(raw string) (int, error) {
	if raw == "" {
		return defaultPageSize, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || !slices.Contains(pageSizes, n) {
		return 0, errors.New("invalid pageSize")
	}
	return n, nil
}

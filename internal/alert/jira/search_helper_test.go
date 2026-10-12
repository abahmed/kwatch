package jira

import (
	"net/http"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// withoutSearch drops the idempotency lookup that precedes a create, so
// lifecycle tests can follow the issue requests themselves.
func withoutSearch(in []providertest.Request) []providertest.Request {
	var out []providertest.Request
	for _, req := range in {
		if req.Method != http.MethodGet {
			out = append(out, req)
		}
	}
	return out
}

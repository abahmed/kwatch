package dashboard

// ListResponse is the /api/incidents response: one page of the
// filtered, sorted list.
//
// Total is how many incidents match every filter and TotalPages how
// many pages of PageSize they fill; Page and PageSize echo the page
// asked for. A page past the last is empty. Facets count the filtered
// set by severity, state, namespace, kind and acknowledgement, each
// ignoring its own filter, so a choice shows what picking it would
// show.
type ListResponse struct {
	Incidents  []IncidentListItem `json:"incidents"`
	Total      int                `json:"total"`
	Page       int                `json:"page"`
	PageSize   int                `json:"pageSize"`
	TotalPages int                `json:"totalPages"`
	Facets     ListFacets         `json:"facets"`
}

// ListFacets are the counts behind the filter chips and pickers, each
// a value and how many issues have it. State ignores status as well,
// so it counts the active and the resolved issues. Namespace and Kind
// hold the 50 and 20 with the most issues; Acked counts acknowledged
// ("true") and not acknowledged ("false") issues.
type ListFacets struct {
	Severity  map[string]int `json:"severity"`
	State     map[string]int `json:"state"`
	Namespace map[string]int `json:"namespace"`
	Kind      map[string]int `json:"kind"`
	Acked     map[string]int `json:"acked"`
}

package dashboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// queryError is a query the dashboard cannot answer: a plain sentence a
// page can show as it is, the query key at fault and the word or value
// that is wrong (the key when the key as a whole is).
type queryError struct {
	msg   string
	param string
	term  string
}

func (e *queryError) Error() string { return e.msg }

// maxErrorTerm bounds the offending value an error echoes: a query
// value can be long, the page only needs enough to point at it.
const maxErrorTerm = 64

// badQuery is a queryError about param; term is the offending value.
// Long strings among args are cut like term.
func badQuery(param, term, format string, args ...any) error {
	if term == "" {
		term = param
	}
	term, _ = shortText(term, maxErrorTerm)
	for i, a := range args {
		if s, ok := a.(string); ok {
			args[i], _ = shortText(s, maxErrorTerm)
		}
	}
	return &queryError{msg: fmt.Sprintf(format, args...), param: param,
		term: term}
}

// queryErrorBody is the 400 body: {"error": sentence, "param": key,
// "term": value}; term is the key when the key as a whole is wrong.
type queryErrorBody struct {
	Error string `json:"error"`
	Param string `json:"param,omitempty"`
	Term  string `json:"term,omitempty"`
}

// writeBadQuery answers 400 for err: a queryError's sentence, key and
// value, or err's text alone.
func writeBadQuery(w http.ResponseWriter, err error) {
	out := queryErrorBody{Error: err.Error()}
	var qe *queryError
	if errors.As(err, &qe) {
		out.Param, out.Term = qe.param, qe.term
	}
	body, _ := json.Marshal(out)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write(append(body, '\n'))
}

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"
)

type Request struct {
	ID       int             `json:"id"`
	Received time.Time       `json:"received"`
	Method   string          `json:"method"`
	Headers  http.Header     `json:"headers"`
	Status   int             `json:"status"`
	Body     []byte          `json:"body"`
	JSON     json.RawMessage `json:"json,omitempty"`
}

type Store struct {
	mu       sync.Mutex
	requests []Request
	policy   Policy
}

func NewStore() *Store {
	return &Store{policy: Policy{Mode: "success"}}
}

func (s *Store) Record(r *http.Request, status int) {
	body, _ := io.ReadAll(r.Body)
	request := Request{
		Received: time.Now().UTC(),
		Method:   r.Method,
		Headers:  r.Header.Clone(),
		Status:   status,
		Body:     append([]byte(nil), body...),
	}
	if json.Valid(body) {
		request.JSON = append([]byte(nil), body...)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	request.ID = len(s.requests) + 1
	s.requests = append(s.requests, request)
}

func (s *Store) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]Request, len(s.requests))
	copy(result, s.requests)
	for i := range result {
		result[i].Body = bytes.Clone(result[i].Body)
		result[i].JSON = bytes.Clone(result[i].JSON)
	}
	return result
}

func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = nil
}

func (s *Store) SetPolicy(policy Policy) error {
	status, _, err := policy.response()
	if err != nil || (status == 0 && policy.Mode != "connection-reset") {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.policy = policy
	return nil
}

func (s *Store) Policy() (int, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status, delay, _ := s.policy.response()
	return status, delay
}

func (s *Store) PolicyState() Policy {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.policy
}

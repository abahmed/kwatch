package main

import (
	"encoding/json"
	"net/http"
	"time"
)

type Server struct {
	store *Store
}

func NewServer() *Server {
	return &Server{store: NewStore()}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/webhook", s.webhook)
	mux.HandleFunc("/requests", s.requests)
	mux.HandleFunc("/control", s.control)
	return mux
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (s *Server) webhook(w http.ResponseWriter, r *http.Request) {
	status, delay := s.store.Policy()
	s.store.Record(r, status)
	if status == 0 {
		hijacker, ok := w.(http.Hijacker)
		if ok {
			connection, _, err := hijacker.Hijack()
			if err == nil {
				_ = connection.Close()
			}
		}
		return
	}
	if delay > 0 {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(delay):
		}
	}
	w.WriteHeader(status)
}

func (s *Server) requests(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		s.store.Clear()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, s.store.Requests())
}

func (s *Server) control(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var policy Policy
		if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.store.SetPolicy(policy); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	writeJSON(w, s.store.PolicyState())
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

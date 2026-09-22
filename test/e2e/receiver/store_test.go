package main

import (
	"net/http/httptest"
	"testing"
)

func TestStoreRecordsOrderedRequests(t *testing.T) {
	store := NewStore()
	for range 2 {
		request := httptest.NewRequest("POST", "/webhook", nil)
		store.Record(request, 200)
	}
	requests := store.Requests()
	if len(requests) != 2 || requests[0].ID != 1 || requests[1].ID != 2 {
		t.Fatalf("unexpected request order: %#v", requests)
	}
}

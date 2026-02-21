package addyio

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func aliasesPage(current, last, count int) aliasesResponse {
	var r aliasesResponse
	r.Meta.CurrentPage = current
	r.Meta.LastPage = last
	for i := 0; i < count; i++ {
		r.Data = append(r.Data, struct {
			ID          string `json:"id"`
			Email       string `json:"email"`
			Active      bool   `json:"active"`
			Description string `json:"description"`
		}{
			ID:    fmt.Sprintf("id-%d-%d", current, i),
			Email: fmt.Sprintf("alias-%d-%d@user.anonaddy.com", current, i),
		})
	}
	return r
}

func TestFetchAliases_SinglePage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(aliasesPage(1, 1, 3))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-key")
	aliases, err := c.FetchAliases()
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(aliases) != 3 {
		t.Errorf("want 3 aliases, got %d", len(aliases))
	}
}

func TestFetchAliases_MultiPage(t *testing.T) {
	var pageNum atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := int(pageNum.Add(1))
		json.NewEncoder(w).Encode(aliasesPage(p, 3, 100))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-key")
	aliases, err := c.FetchAliases()
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(aliases) != 300 {
		t.Errorf("want 300 aliases, got %d", len(aliases))
	}
}

func TestFetchAliases_RateLimit(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n < 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		json.NewEncoder(w).Encode(aliasesPage(1, 1, 2))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-key")
	aliases, err := c.FetchAliases()
	if err != nil {
		t.Fatalf("fetch after retry: %v", err)
	}
	if len(aliases) != 2 {
		t.Errorf("want 2 aliases, got %d", len(aliases))
	}
}

func TestFetchAliases_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-key")
	_, err := c.FetchAliases()
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

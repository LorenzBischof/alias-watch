package notify

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSend_DisabledWhenURLMissing(t *testing.T) {
	c := NewClient("", "my-secret-token")
	err := c.Send(Notification{
		Kind:          "new",
		Alias:         "abc@user.anonaddy.com",
		Account:       "GitHub",
		Sender:        "noreply@github.com",
		Subject:       "PR merged",
		Domain:        "github.com",
		DomainContext: "known — 2 addresses seen",
	})
	if err != nil {
		t.Fatalf("send should be disabled without url, got: %v", err)
	}
}

func TestSend_NewSender(t *testing.T) {
	var gotReq *http.Request
	var gotBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReq = r
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "")
	err := c.Send(Notification{
		Kind:          "new",
		Alias:         "abc@user.anonaddy.com",
		Account:       "GitHub",
		Sender:        "noreply@github.com",
		Subject:       "PR merged",
		Domain:        "github.com",
		DomainContext: "known — 2 addresses seen",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if gotReq == nil {
		t.Fatal("no request received")
	}
	if got := gotReq.Header.Get("Title"); !strings.Contains(got, "New sender") {
		t.Errorf("title should contain 'New sender', got %q", got)
	}
	if gotReq.Header.Get("Priority") != "high" {
		t.Errorf("priority should be high, got %q", gotReq.Header.Get("Priority"))
	}
	if gotReq.Header.Get("Tags") != "warning" {
		t.Errorf("tags should be warning, got %q", gotReq.Header.Get("Tags"))
	}
	if !strings.Contains(gotBody, "noreply@github.com") {
		t.Errorf("body missing sender, got: %s", gotBody)
	}
	if !strings.Contains(gotBody, "GitHub") {
		t.Errorf("body missing account, got: %s", gotBody)
	}
	if !strings.Contains(gotBody, "Alias status: existing") {
		t.Errorf("body missing alias status for existing alias, got: %s", gotBody)
	}
}

func TestSend_FlaggedSender(t *testing.T) {
	var gotTitle string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTitle = r.Header.Get("Title")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "")
	err := c.Send(Notification{
		Kind:          "flagged",
		Alias:         "abc@user.anonaddy.com",
		Account:       "Amazon",
		Sender:        "phish@evil.com",
		Subject:       "You won",
		Domain:        "evil.com",
		DomainContext: "new domain",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if !strings.Contains(gotTitle, "Flagged sender") {
		t.Errorf("title should contain 'Flagged sender', got %q", gotTitle)
	}
}

func TestSend_WithToken(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "my-secret-token")
	err := c.Send(Notification{
		Kind:          "new",
		Alias:         "abc@user.anonaddy.com",
		Account:       "GitHub",
		Sender:        "noreply@github.com",
		Subject:       "test",
		Domain:        "github.com",
		DomainContext: "new domain",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if gotAuth != "Bearer my-secret-token" {
		t.Errorf("expected Bearer token, got %q", gotAuth)
	}
}

func TestSend_NewAliasAndSender(t *testing.T) {
	var gotTitle string
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTitle = r.Header.Get("Title")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "")
	err := c.Send(Notification{
		Kind:          "new",
		Alias:         "new@user.anonaddy.com",
		Account:       "UNMAPPED",
		Sender:        "noreply@newservice.com",
		Subject:       "Welcome",
		Domain:        "newservice.com",
		AliasIsNew:    true,
		DomainContext: "new domain",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if !strings.Contains(gotTitle, "New alias + sender") {
		t.Errorf("title should contain 'New alias + sender', got %q", gotTitle)
	}
	if !strings.Contains(gotBody, "Alias status: new (auto-added)") {
		t.Errorf("body missing alias new marker, got: %s", gotBody)
	}
}

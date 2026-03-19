package imap

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	imapclient "github.com/emersion/go-imap/v2/imapclient"
	"github.com/lorenzbischof/alias-watch/internal/db"
	"github.com/lorenzbischof/alias-watch/internal/notify"
)

func TestProcessNewMessage_AutoAddsAliasWithTitleAndSendsDedicatedNotification(t *testing.T) {
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	var gotTitle string
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTitle = r.Header.Get("Title")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	opts := MonitorOptions{
		Store:    store,
		Notifier: notify.NewClient(srv.URL, ""),
	}

	headers := map[string][]string{
		"To":                         {"new-alias@user.anonaddy.com"},
		"X-Anonaddy-Original-Sender": {"welcome@newservice.com"},
		"Subject":                    {"Welcome"},
		"Message-Id":                 {"<msg-auto-alias>"},
	}
	processNewMessage(context.Background(), opts, headers)

	aliases, err := store.AllAliases()
	if err != nil {
		t.Fatalf("all aliases: %v", err)
	}
	if len(aliases) != 1 {
		t.Fatalf("want 1 alias, got %d", len(aliases))
	}
	if aliases[0].Email != "new-alias@user.anonaddy.com" {
		t.Fatalf("unexpected alias email: %q", aliases[0].Email)
	}
	if aliases[0].Title != autoAddedAliasTitle {
		t.Fatalf("want auto-added alias title %q, got %q", autoAddedAliasTitle, aliases[0].Title)
	}

	senders, err := store.KnownSendersForAlias("new-alias@user.anonaddy.com")
	if err != nil {
		t.Fatalf("known senders: %v", err)
	}
	if len(senders) != 1 {
		t.Fatalf("want 1 known sender, got %d", len(senders))
	}

	if !strings.Contains(gotTitle, "New alias + sender") {
		t.Fatalf("expected dedicated title for new alias + sender, got %q", gotTitle)
	}
	if !strings.Contains(gotBody, "Alias status: new (auto-added)") {
		t.Fatalf("expected alias status marker in body, got %q", gotBody)
	}
	if !strings.Contains(gotBody, "History: none (0)") {
		t.Fatalf("expected empty history context in body, got %q", gotBody)
	}
}

func TestProcessNewMessage_NewSenderOnExistingAliasUsesExistingAliasNotification(t *testing.T) {
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	if err := store.UpsertAlias(db.Alias{
		Email:    "existing@user.anonaddy.com",
		AddyID:   "seed-1",
		Active:   true,
		Title:    "GitHub",
		SyncedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed alias: %v", err)
	}
	now := time.Now().UTC()
	seedEmails := []db.Email{
		{
			AliasEmail: "existing@user.anonaddy.com",
			FromAddr:   "noreply@github.com",
			Subject:    "Seed 1",
			ReceivedAt: now.Add(-5 * time.Minute),
			MessageID:  "seed-1",
		},
		{
			AliasEmail: "existing@user.anonaddy.com",
			FromAddr:   "noreply@github.com",
			Subject:    "Seed 2",
			ReceivedAt: now.Add(-4 * time.Minute),
			MessageID:  "seed-2",
		},
		{
			AliasEmail: "existing@user.anonaddy.com",
			FromAddr:   "status@github.com",
			Subject:    "Seed 3",
			ReceivedAt: now.Add(-3 * time.Minute),
			MessageID:  "seed-3",
		},
	}
	for _, e := range seedEmails {
		if _, err := store.InsertEmail(e); err != nil {
			t.Fatalf("seed email: %v", err)
		}
	}

	var gotTitle string
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTitle = r.Header.Get("Title")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	opts := MonitorOptions{
		Store:    store,
		Notifier: notify.NewClient(srv.URL, ""),
	}

	headers := map[string][]string{
		"To":                         {"existing@user.anonaddy.com"},
		"X-Anonaddy-Original-Sender": {"security@github.com"},
		"Subject":                    {"Sign-in detected"},
		"Message-Id":                 {"<msg-existing-alias>"},
	}
	processNewMessage(context.Background(), opts, headers)

	aliases, err := store.AllAliases()
	if err != nil {
		t.Fatalf("all aliases: %v", err)
	}
	if len(aliases) != 1 {
		t.Fatalf("want 1 alias, got %d", len(aliases))
	}
	if aliases[0].Title != "GitHub" {
		t.Fatalf("existing alias title should be unchanged, got %q", aliases[0].Title)
	}

	if !strings.Contains(gotTitle, "New sender for existing@user.anonaddy.com") {
		t.Fatalf("expected existing-alias new-sender title, got %q", gotTitle)
	}
	if !strings.Contains(gotBody, "Alias status: existing") {
		t.Fatalf("expected existing alias status in body, got %q", gotBody)
	}
	if !strings.Contains(gotBody, "History: noreply@github.com (2), github.com (3)") {
		t.Fatalf("expected history context in body, got %q", gotBody)
	}
}

func TestIdleCounter_IgnoresUnilateralMailboxUpdates(t *testing.T) {
	counter := newIdleCounter(12)
	next := uint32(13)

	counter.OnMailbox(&imapclient.UnilateralDataMailbox{NumMessages: &next})

	if counter.Baseline() != 12 {
		t.Fatalf("baseline should remain unchanged, got %d", counter.Baseline())
	}
}

func TestIdleCounter_WakesOnMailboxUpdate(t *testing.T) {
	counter := newIdleCounter(1)
	next := uint32(2)

	select {
	case <-counter.WakeCh():
		t.Fatal("did not expect wake before mailbox update")
	default:
	}

	counter.OnMailbox(&imapclient.UnilateralDataMailbox{NumMessages: &next})

	select {
	case <-counter.WakeCh():
	default:
		t.Fatal("expected wake after mailbox update")
	}
}

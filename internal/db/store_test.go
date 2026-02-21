package db

import (
	"testing"
	"time"
)

func openTestDB(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestUpsertAlias(t *testing.T) {
	s := openTestDB(t)
	now := time.Now().UTC().Truncate(time.Second)

	a := Alias{
		Email:       "abc@user.anonaddy.com",
		AddyID:      "addy-123",
		Active:      true,
		Description: "test",
		SyncedAt:    now,
	}
	if err := s.UpsertAlias(a); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	aliases, err := s.AllAliases()
	if err != nil {
		t.Fatalf("all aliases: %v", err)
	}
	if len(aliases) != 1 {
		t.Fatalf("want 1 alias, got %d", len(aliases))
	}
	got := aliases[0]
	if got.Email != a.Email || got.AddyID != a.AddyID || !got.Active {
		t.Errorf("unexpected alias: %+v", got)
	}

	// Update
	a.Description = "updated"
	if err := s.UpsertAlias(a); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	aliases, _ = s.AllAliases()
	if aliases[0].Description != "updated" {
		t.Errorf("description not updated")
	}
}

func TestAliasAccounts_Multiple(t *testing.T) {
	s := openTestDB(t)
	alias := "abc@user.anonaddy.com"

	s.UpsertAlias(Alias{Email: alias, AddyID: "1", Active: true, SyncedAt: time.Now()})
	if err := s.ReplaceAliasAccounts(alias, []string{"GitHub", "GitHub Enterprise"}); err != nil {
		t.Fatalf("replace: %v", err)
	}

	accounts, err := s.AccountsForAlias(alias)
	if err != nil {
		t.Fatalf("accounts: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("want 2 accounts, got %d", len(accounts))
	}
}

func TestAliasAccounts_Sync(t *testing.T) {
	s := openTestDB(t)
	alias := "abc@user.anonaddy.com"

	s.UpsertAlias(Alias{Email: alias, AddyID: "1", Active: true, SyncedAt: time.Now()})
	s.ReplaceAliasAccounts(alias, []string{"GitHub", "GitLab"})

	// Sync removes GitLab, keeps GitHub
	if err := s.ReplaceAliasAccounts(alias, []string{"GitHub"}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	accounts, _ := s.AccountsForAlias(alias)
	if len(accounts) != 1 || accounts[0] != "GitHub" {
		t.Errorf("want [GitHub], got %v", accounts)
	}
}

func TestSenderLookup_EmailMatch(t *testing.T) {
	s := openTestDB(t)
	now := time.Now()
	alias := "abc@user.anonaddy.com"

	ks := KnownSender{
		AliasEmail:   alias,
		SenderEmail:  "noreply@github.com",
		SenderDomain: "github.com",
		FirstSeen:    now,
		LastSeen:     now,
	}
	if _, err := s.UpsertKnownSender(ks); err != nil {
		t.Fatalf("upsert sender: %v", err)
	}

	senders, err := s.KnownSendersForAlias(alias)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if len(senders) != 1 {
		t.Fatalf("want 1 sender, got %d", len(senders))
	}
	if senders[0].SenderEmail != "noreply@github.com" {
		t.Errorf("wrong sender: %v", senders[0].SenderEmail)
	}
}

func TestKnownDomainsForAlias(t *testing.T) {
	s := openTestDB(t)
	now := time.Now()
	alias := "abc@user.anonaddy.com"

	if err := s.UpsertKnownDomain(KnownDomain{
		AliasEmail:   alias,
		SenderDomain: "github.com",
		Enabled:      true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("upsert domain: %v", err)
	}

	domains, err := s.KnownDomainsForAlias(alias)
	if err != nil {
		t.Fatalf("lookup domains: %v", err)
	}
	if len(domains) != 1 {
		t.Fatalf("want 1 domain, got %d", len(domains))
	}
	if domains[0].SenderDomain != "github.com" || !domains[0].Enabled {
		t.Fatalf("unexpected domain rule: %+v", domains[0])
	}
}

func TestSenderLookup_Miss(t *testing.T) {
	s := openTestDB(t)
	alias := "abc@user.anonaddy.com"

	senders, err := s.KnownSendersForAlias(alias)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if len(senders) != 0 {
		t.Errorf("want 0 senders, got %d", len(senders))
	}
}

func TestFlagEmail(t *testing.T) {
	s := openTestDB(t)
	now := time.Now()
	alias := "abc@user.anonaddy.com"
	sender := "phish@evil.com"

	s.UpsertKnownSender(KnownSender{
		AliasEmail:   alias,
		SenderEmail:  sender,
		SenderDomain: "evil.com",
		FirstSeen:    now,
		LastSeen:     now,
	})

	id, err := s.InsertEmail(Email{
		AliasEmail: alias,
		FromAddr:   sender,
		Subject:    "You won!",
		ReceivedAt: now,
		MessageID:  "msg-1",
	})
	if err != nil {
		t.Fatalf("insert email: %v", err)
	}

	if err := s.FlagEmail(id); err != nil {
		t.Fatalf("flag: %v", err)
	}

	emails, _ := s.EmailsForAlias(alias)
	if len(emails) != 1 || !emails[0].Flagged {
		t.Errorf("email not flagged")
	}

	senders, _ := s.KnownSendersForAlias(alias)
	if len(senders) != 1 || !senders[0].Flagged {
		t.Errorf("known_sender not flagged")
	}
}

func TestDeleteKnownSender(t *testing.T) {
	s := openTestDB(t)
	now := time.Now()
	alias := "abc@user.anonaddy.com"

	ks := KnownSender{
		AliasEmail:   alias,
		SenderEmail:  "noreply@github.com",
		SenderDomain: "github.com",
		FirstSeen:    now,
		LastSeen:     now,
	}
	if _, err := s.UpsertKnownSender(ks); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	senders, err := s.KnownSendersForAlias(alias)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if len(senders) != 1 {
		t.Fatalf("want 1 sender before delete, got %d", len(senders))
	}

	if err := s.DeleteKnownSender(senders[0].ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	senders, err = s.KnownSendersForAlias(alias)
	if err != nil {
		t.Fatalf("lookup after delete: %v", err)
	}
	if len(senders) != 0 {
		t.Errorf("want 0 senders after delete, got %d", len(senders))
	}
}

func TestUpdateKnownSender(t *testing.T) {
	s := openTestDB(t)
	now := time.Now()
	alias := "abc@user.anonaddy.com"

	ks := KnownSender{
		AliasEmail:   alias,
		SenderEmail:  "noreply@github.com",
		SenderDomain: "github.com",
		Flagged:      false,
		FirstSeen:    now,
		LastSeen:     now,
	}
	if _, err := s.UpsertKnownSender(ks); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	senders, err := s.KnownSendersForAlias(alias)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if len(senders) != 1 {
		t.Fatalf("want 1 sender, got %d", len(senders))
	}

	updated := senders[0]
	updated.Flagged = true

	if err := s.UpdateKnownSender(updated); err != nil {
		t.Fatalf("update: %v", err)
	}

	senders, err = s.KnownSendersForAlias(alias)
	if err != nil {
		t.Fatalf("lookup after update: %v", err)
	}
	if len(senders) != 1 {
		t.Fatalf("want 1 sender after update, got %d", len(senders))
	}
	got := senders[0]
	if !got.Flagged {
		t.Errorf("want flagged=true, got false")
	}
}

func TestDomainFamiliarity(t *testing.T) {
	s := openTestDB(t)
	now := time.Now()
	alias := "abc@user.anonaddy.com"

	senders := []KnownSender{
		{AliasEmail: alias, SenderEmail: "a@github.com", SenderDomain: "github.com", FirstSeen: now, LastSeen: now},
		{AliasEmail: alias, SenderEmail: "b@github.com", SenderDomain: "github.com", FirstSeen: now, LastSeen: now},
		{AliasEmail: alias, SenderEmail: "c@other.com", SenderDomain: "other.com", FirstSeen: now, LastSeen: now},
	}
	for _, ks := range senders {
		s.UpsertKnownSender(ks)
	}

	count, err := s.CountKnownSendersByDomain(alias, "github.com")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Errorf("want 2, got %d", count)
	}

	count, _ = s.CountKnownSendersByDomain(alias, "other.com")
	if count != 1 {
		t.Errorf("want 1 for other.com, got %d", count)
	}

	count, _ = s.CountKnownSendersByDomain(alias, "unknown.com")
	if count != 0 {
		t.Errorf("want 0 for unknown domain, got %d", count)
	}
}

func TestAliasLastSeen(t *testing.T) {
	s := openTestDB(t)
	base := time.Now().UTC().Truncate(time.Second)

	if _, err := s.UpsertKnownSender(KnownSender{
		AliasEmail:   "a@user.anonaddy.com",
		SenderEmail:  "x@one.com",
		SenderDomain: "one.com",
		FirstSeen:    base,
		LastSeen:     base.Add(-2 * time.Hour),
	}); err != nil {
		t.Fatalf("upsert sender 1: %v", err)
	}
	if _, err := s.UpsertKnownSender(KnownSender{
		AliasEmail:   "a@user.anonaddy.com",
		SenderEmail:  "y@two.com",
		SenderDomain: "two.com",
		FirstSeen:    base,
		LastSeen:     base.Add(-1 * time.Hour),
	}); err != nil {
		t.Fatalf("upsert sender 2: %v", err)
	}
	if _, err := s.UpsertKnownSender(KnownSender{
		AliasEmail:   "b@user.anonaddy.com",
		SenderEmail:  "z@three.com",
		SenderDomain: "three.com",
		FirstSeen:    base,
		LastSeen:     base.Add(-3 * time.Hour),
	}); err != nil {
		t.Fatalf("upsert sender 3: %v", err)
	}

	got, err := s.AliasLastSeen()
	if err != nil {
		t.Fatalf("alias last seen: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 aliases, got %d", len(got))
	}
	if !got["a@user.anonaddy.com"].Equal(base.Add(-1 * time.Hour)) {
		t.Fatalf("wrong latest ts for alias a: %v", got["a@user.anonaddy.com"])
	}
	if !got["b@user.anonaddy.com"].Equal(base.Add(-3 * time.Hour)) {
		t.Fatalf("wrong latest ts for alias b: %v", got["b@user.anonaddy.com"])
	}
}

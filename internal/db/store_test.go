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
		Email:    "abc@user.anonaddy.com",
		AddyID:   "addy-123",
		Active:   true,
		Title:    "test",
		SyncedAt: now,
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

	// Re-sync with a different title: existing non-empty value must be preserved.
	a.Title = "from addy.io again"
	if err := s.UpsertAlias(a); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	aliases, _ = s.AllAliases()
	if aliases[0].Title != "test" {
		t.Errorf("title should be preserved on re-sync, got %q", aliases[0].Title)
	}
}

func TestAliasExists(t *testing.T) {
	s := openTestDB(t)
	alias := "abc@user.anonaddy.com"

	exists, err := s.AliasExists(alias)
	if err != nil {
		t.Fatalf("alias exists (before): %v", err)
	}
	if exists {
		t.Fatalf("expected alias to not exist before upsert")
	}

	if err := s.UpsertAlias(Alias{
		Email:    alias,
		AddyID:   "addy-123",
		Active:   true,
		SyncedAt: time.Now(),
	}); err != nil {
		t.Fatalf("upsert alias: %v", err)
	}

	exists, err = s.AliasExists(alias)
	if err != nil {
		t.Fatalf("alias exists (after): %v", err)
	}
	if !exists {
		t.Fatalf("expected alias to exist after upsert")
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

func TestUpdateAliasTitle(t *testing.T) {
	s := openTestDB(t)
	alias := "abc@user.anonaddy.com"

	if err := s.UpsertAlias(Alias{Email: alias, AddyID: "1", Active: true, Title: "original", SyncedAt: time.Now()}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if err := s.UpdateAliasTitle(alias, "edited locally"); err != nil {
		t.Fatalf("update title: %v", err)
	}

	// Sync should NOT overwrite the locally-edited title.
	if err := s.UpsertAlias(Alias{Email: alias, AddyID: "1", Active: true, Title: "from addy.io", SyncedAt: time.Now()}); err != nil {
		t.Fatalf("upsert after edit: %v", err)
	}

	aliases, err := s.AllAliases()
	if err != nil {
		t.Fatalf("all aliases: %v", err)
	}
	if len(aliases) != 1 {
		t.Fatalf("want 1 alias, got %d", len(aliases))
	}
	if aliases[0].Title != "edited locally" {
		t.Errorf("want title 'edited locally', got %q", aliases[0].Title)
	}
}

func TestUpsertAlias_PopulatesEmptyTitle(t *testing.T) {
	s := openTestDB(t)
	alias := "abc@user.anonaddy.com"

	// First sync: empty title → should be populated from supplied value.
	if err := s.UpsertAlias(Alias{Email: alias, AddyID: "1", Active: true, Title: "", SyncedAt: time.Now()}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	// Second sync: supplies a value for an alias that had empty title.
	if err := s.UpsertAlias(Alias{Email: alias, AddyID: "1", Active: true, Title: "GitHub", SyncedAt: time.Now()}); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	aliases, err := s.AllAliases()
	if err != nil {
		t.Fatalf("all aliases: %v", err)
	}
	if aliases[0].Title != "GitHub" {
		t.Errorf("want title 'GitHub', got %q", aliases[0].Title)
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

func TestMostUsedSenderAndDomainForAlias(t *testing.T) {
	s := openTestDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	alias := "abc@user.anonaddy.com"

	emails := []Email{
		{AliasEmail: alias, FromAddr: "a@github.com", Subject: "1", ReceivedAt: now.Add(-5 * time.Minute), MessageID: "m1"},
		{AliasEmail: alias, FromAddr: "a@github.com", Subject: "2", ReceivedAt: now.Add(-4 * time.Minute), MessageID: "m2"},
		{AliasEmail: alias, FromAddr: "noreply@alerts.github.com", Subject: "3", ReceivedAt: now.Add(-3 * time.Minute), MessageID: "m3"},
		{AliasEmail: alias, FromAddr: "b@other.com", Subject: "4", ReceivedAt: now.Add(-2 * time.Minute), MessageID: "m4"},
		{AliasEmail: "other@user.anonaddy.com", FromAddr: "z@elsewhere.com", Subject: "x", ReceivedAt: now.Add(-1 * time.Minute), MessageID: "m5"},
	}
	for _, e := range emails {
		if _, err := s.InsertEmail(e); err != nil {
			t.Fatalf("insert email: %v", err)
		}
	}

	sender, senderCount, err := s.MostUsedSenderForAlias(alias)
	if err != nil {
		t.Fatalf("most used sender: %v", err)
	}
	if sender != "a@github.com" || senderCount != 2 {
		t.Fatalf("want sender a@github.com (2), got %q (%d)", sender, senderCount)
	}

	domain, domainCount, err := s.MostUsedDomainForAlias(alias)
	if err != nil {
		t.Fatalf("most used domain: %v", err)
	}
	if domain != "github.com" || domainCount != 2 {
		t.Fatalf("want domain github.com (2), got %q (%d)", domain, domainCount)
	}
}

func TestMostUsedSenderAndDomainForAlias_Empty(t *testing.T) {
	s := openTestDB(t)

	sender, senderCount, err := s.MostUsedSenderForAlias("none@user.anonaddy.com")
	if err != nil {
		t.Fatalf("most used sender: %v", err)
	}
	if sender != "" || senderCount != 0 {
		t.Fatalf("want empty sender stats, got %q (%d)", sender, senderCount)
	}

	domain, domainCount, err := s.MostUsedDomainForAlias("none@user.anonaddy.com")
	if err != nil {
		t.Fatalf("most used domain: %v", err)
	}
	if domain != "" || domainCount != 0 {
		t.Fatalf("want empty domain stats, got %q (%d)", domain, domainCount)
	}
}

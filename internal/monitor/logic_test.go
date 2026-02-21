package monitor

import (
	"testing"

	"github.com/lorenzbischof/email-monitoring/internal/db"
)

func TestIsKnownSender_ExactMatch(t *testing.T) {
	senders := []db.KnownSender{
		{SenderEmail: "noreply@github.com", SenderDomain: "github.com", Flagged: false},
	}
	found, flagged := IsKnownSender(senders, nil, "noreply@github.com", "github.com")
	if !found {
		t.Error("expected found=true")
	}
	if flagged {
		t.Error("expected flagged=false")
	}
}

func TestIsKnownSender_DomainMatch(t *testing.T) {
	domains := []db.KnownDomain{
		{SenderDomain: "github.com", Enabled: true},
	}
	// Different email address, same domain
	found, flagged := IsKnownSender(nil, domains, "security@github.com", "github.com")
	if !found {
		t.Error("expected found=true for domain match")
	}
	if flagged {
		t.Error("expected flagged=false")
	}
}

func TestIsKnownSender_Flagged(t *testing.T) {
	senders := []db.KnownSender{
		{SenderEmail: "phish@evil.com", SenderDomain: "evil.com", Flagged: true},
	}
	found, flagged := IsKnownSender(senders, nil, "phish@evil.com", "evil.com")
	if !found {
		t.Error("expected found=true")
	}
	if !flagged {
		t.Error("expected flagged=true")
	}
}

func TestIsKnownSender_Miss(t *testing.T) {
	senders := []db.KnownSender{
		{SenderEmail: "noreply@github.com", SenderDomain: "github.com", Flagged: false},
	}
	// Completely different sender
	found, _ := IsKnownSender(senders, nil, "attacker@evil.com", "evil.com")
	if found {
		t.Error("expected found=false")
	}
}

func TestIsKnownSender_DomainRuleDisabled(t *testing.T) {
	domains := []db.KnownDomain{
		{SenderDomain: "github.com", Enabled: false},
	}
	found, _ := IsKnownSender(nil, domains, "security@github.com", "github.com")
	if found {
		t.Error("expected found=false when domain rule is disabled")
	}
}

func TestIsKnownSender_ExactMatchTakesPrecedenceOverDomainRule(t *testing.T) {
	senders := []db.KnownSender{
		{SenderEmail: "security@github.com", SenderDomain: "github.com", Flagged: true},
	}
	domains := []db.KnownDomain{
		{SenderDomain: "github.com", Enabled: true},
	}
	found, flagged := IsKnownSender(senders, domains, "security@github.com", "github.com")
	if !found {
		t.Error("expected found=true")
	}
	if !flagged {
		t.Error("expected flagged=true from exact sender, not domain rule")
	}
}

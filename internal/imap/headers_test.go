package imap

import "testing"

func TestExtractAliasEmail_UsesXOriginalToFallback(t *testing.T) {
	headers := map[string][]string{
		"X-Original-To": {"new-alias@user.anonaddy.com"},
	}

	got := ExtractAliasEmail(headers)
	if got != "new-alias@user.anonaddy.com" {
		t.Fatalf("expected alias from X-Original-To, got %q", got)
	}
}

func TestExtractAliasEmail_UsesEnvelopeToFallback(t *testing.T) {
	headers := map[string][]string{
		"Envelope-To": {"second-alias@user.anonaddy.com"},
	}

	got := ExtractAliasEmail(headers)
	if got != "second-alias@user.anonaddy.com" {
		t.Fatalf("expected alias from Envelope-To, got %q", got)
	}
}

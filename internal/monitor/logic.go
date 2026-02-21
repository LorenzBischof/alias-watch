package monitor

import "github.com/lorenzbischof/email-monitoring/internal/db"

// IsKnownSender checks the list of known senders and alias-level domains for a
// match against the incoming email address and domain. It returns:
//   - found=true if the sender is recognised (exact email first, then domain)
//   - flagged=true if the exact sender has been manually flagged
func IsKnownSender(senders []db.KnownSender, domains []db.KnownDomain, email, domain string) (found bool, flagged bool) {
	for _, s := range senders {
		if s.SenderEmail == email {
			return true, s.Flagged
		}
	}
	for _, d := range domains {
		if d.Enabled && d.SenderDomain == domain {
			return true, false
		}
	}
	return false, false
}

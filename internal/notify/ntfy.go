package notify

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Client sends notifications to ntfy.sh.
type Client struct {
	NtfyURL   string
	NtfyToken string
	HTTP      *http.Client
}

// NewClient creates a new ntfy notification client.
func NewClient(ntfyURL, ntfyToken string) *Client {
	return &Client{
		NtfyURL:   ntfyURL,
		NtfyToken: ntfyToken,
		HTTP:      &http.Client{Timeout: 10 * time.Second},
	}
}

// Notification represents an alert to send.
type Notification struct {
	// Title prefix: "New sender" or "Flagged sender"
	Kind    string // "new" or "flagged"
	Alias   string
	Account string
	Sender  string
	Subject string
	Domain  string
	// True when alias was auto-created during this event.
	AliasIsNew    bool
	DomainContext string // e.g. "known — 3 addresses seen" or "new domain"
	TopSender     string
	TopSenderHits int
	TopDomain     string
	TopDomainHits int
}

// Send posts a notification to ntfy.sh.
func (c *Client) Send(n Notification) error {
	if strings.TrimSpace(c.NtfyURL) == "" {
		return nil
	}

	titlePrefix := "New sender"
	if n.Kind == "flagged" {
		titlePrefix = "Flagged sender"
	} else if n.AliasIsNew {
		titlePrefix = "New alias + sender"
	}
	title := fmt.Sprintf("%s for %s", titlePrefix, n.Alias)

	aliasStatus := "existing"
	if n.AliasIsNew {
		aliasStatus = "new (auto-added)"
	}

	historyLine := "none (0)"
	if n.TopSenderHits > 0 || n.TopDomainHits > 0 {
		historyLine = fmt.Sprintf("%s (%d), %s (%d)",
			valueOrUnknown(n.TopSender), n.TopSenderHits, valueOrUnknown(n.TopDomain), n.TopDomainHits)
	}

	body := fmt.Sprintf("From:    %s\nAlias:   %s  →  %s\nAlias status: %s\nSubject: %s\nDomain:  %s (%s)\nHistory: %s",
		n.Sender, n.Alias, n.Account, aliasStatus, n.Subject, n.Domain, n.DomainContext, historyLine)

	req, err := http.NewRequest(http.MethodPost, c.NtfyURL, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Title", title)
	req.Header.Set("Priority", "high")
	req.Header.Set("Tags", "warning")
	if c.NtfyToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.NtfyToken)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("send notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy returned %d", resp.StatusCode)
	}
	return nil
}

func valueOrUnknown(v string) string {
	if strings.TrimSpace(v) == "" {
		return "unknown"
	}
	return v
}

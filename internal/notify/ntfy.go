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
	Kind     string // "new" or "flagged"
	Alias    string
	Account  string
	Sender   string
	Subject  string
	Domain   string
	DomainContext string // e.g. "known — 3 addresses seen" or "new domain"
}

// Send posts a notification to ntfy.sh.
func (c *Client) Send(n Notification) error {
	titlePrefix := "New sender"
	if n.Kind == "flagged" {
		titlePrefix = "Flagged sender"
	}
	title := fmt.Sprintf("%s for %s", titlePrefix, n.Alias)

	body := fmt.Sprintf("From:    %s\nAlias:   %s  →  %s\nSubject: %s\nDomain:  %s (%s)",
		n.Sender, n.Alias, n.Account, n.Subject, n.Domain, n.DomainContext)

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

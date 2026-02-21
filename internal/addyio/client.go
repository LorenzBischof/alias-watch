package addyio

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Alias represents an addy.io alias from the API.
type Alias struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	Active      bool   `json:"active"`
	Description string `json:"description"`
}

// Client is an addy.io API client.
type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// NewClient creates a new addy.io client.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL:    baseURL,
		APIKey:     apiKey,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

type aliasesResponse struct {
	Data []struct {
		ID          string `json:"id"`
		Email       string `json:"email"`
		Active      bool   `json:"active"`
		Description string `json:"description"`
	} `json:"data"`
	Meta struct {
		CurrentPage int `json:"current_page"`
		LastPage    int `json:"last_page"`
	} `json:"meta"`
	Links struct {
		Next string `json:"next"`
	} `json:"links"`
}

// FetchAliases retrieves all aliases via paginated API calls.
func (c *Client) FetchAliases() ([]Alias, error) {
	var all []Alias
	page := 1

	for {
		u, err := url.Parse(c.BaseURL + "/api/v1/aliases")
		if err != nil {
			return nil, fmt.Errorf("parse url: %w", err)
		}
		q := u.Query()
		q.Set("page[size]", "100")
		q.Set("page[number]", fmt.Sprintf("%d", page))
		u.RawQuery = q.Encode()

		var resp aliasesResponse
		if err := c.get(u.String(), &resp); err != nil {
			return nil, err
		}

		for _, d := range resp.Data {
			all = append(all, Alias{
				ID:          d.ID,
				Email:       d.Email,
				Active:      d.Active,
				Description: d.Description,
			})
		}

		if page >= resp.Meta.LastPage || resp.Meta.LastPage == 0 {
			break
		}
		page++
	}

	return all, nil
}

func (c *Client) get(rawURL string, out interface{}) error {
	const maxRetries = 3
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		req, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("http get: %w", err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			lastErr = fmt.Errorf("rate limited (429)")
			continue
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("http %d: %s", resp.StatusCode, string(body))
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read body: %w", err)
		}
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("decode json: %w", err)
		}
		return nil
	}

	return lastErr
}

package imap

import (
	"fmt"

	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/lorenzbischof/alias-watch/internal/config"
)

// Connect establishes an IMAP connection and logs in.
func Connect(cfg config.IMAPConfig) (*imapclient.Client, error) {
	addr := fmt.Sprintf("%s:%d", cfg.Server, cfg.Port)

	var c *imapclient.Client
	var err error

	opts := &imapclient.Options{}

	if cfg.TLS {
		c, err = imapclient.DialTLS(addr, opts)
	} else {
		c, err = imapclient.DialInsecure(addr, opts)
	}
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}

	if err := c.Login(cfg.Username, cfg.Password()).Wait(); err != nil {
		c.Close()
		return nil, fmt.Errorf("imap login: %w", err)
	}

	return c, nil
}

package imap

import (
	"context"
	"fmt"
	"strings"
	"time"

	imap "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/lorenzbischof/email-monitoring/internal/config"
	"github.com/lorenzbischof/email-monitoring/internal/db"
	"github.com/lorenzbischof/email-monitoring/internal/monitor"
	"github.com/lorenzbischof/email-monitoring/internal/notify"
)

const idleInterval = 25 * time.Minute

// MonitorOptions holds configuration for the IDLE monitor.
type MonitorOptions struct {
	IMAPConfig  config.IMAPConfig
	Store       *db.Store
	Notifier    *notify.Client
	AliasDomain string
}

// Monitor runs the IDLE daemon until ctx is cancelled.
func Monitor(ctx context.Context, opts MonitorOptions) error {
	backoff := time.Second
	const maxBackoff = 5 * time.Minute

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err := runIDLESession(ctx, opts); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			fmt.Printf("IDLE session error: %v — reconnecting in %v\n", err, backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		} else {
			backoff = time.Second
		}
	}
}

func runIDLESession(ctx context.Context, opts MonitorOptions) error {
	var lastKnownCount uint32

	clientOpts := &imapclient.Options{
		UnilateralDataHandler: &imapclient.UnilateralDataHandler{
			Mailbox: func(data *imapclient.UnilateralDataMailbox) {
				if data.NumMessages != nil {
					lastKnownCount = *data.NumMessages
				}
			},
		},
	}

	addr := fmt.Sprintf("%s:%d", opts.IMAPConfig.Server, opts.IMAPConfig.Port)
	var (
		client *imapclient.Client
		err    error
	)
	if opts.IMAPConfig.TLS {
		client, err = imapclient.DialTLS(addr, clientOpts)
	} else {
		client, err = imapclient.DialInsecure(addr, clientOpts)
	}
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer client.Close()

	if err := client.Login(opts.IMAPConfig.Username, opts.IMAPConfig.Password()).Wait(); err != nil {
		return fmt.Errorf("login: %w", err)
	}

	selected, err := client.Select(opts.IMAPConfig.Folder, nil).Wait()
	if err != nil {
		return fmt.Errorf("select: %w", err)
	}
	lastKnownCount = selected.NumMessages

	for {
		if ctx.Err() != nil {
			return nil
		}

		// Enter IDLE
		idleCmd, err := client.Idle()
		if err != nil {
			return fmt.Errorf("idle: %w", err)
		}

		// Wait up to 25 minutes or until context cancels
		idleTimer := time.NewTimer(idleInterval)
		select {
		case <-idleTimer.C:
		case <-ctx.Done():
			idleTimer.Stop()
		}

		if err := idleCmd.Close(); err != nil {
			return fmt.Errorf("close idle: %w", err)
		}

		if ctx.Err() != nil {
			return nil
		}

		// Check for new messages
		status, err := client.Status(opts.IMAPConfig.Folder, &imap.StatusOptions{NumMessages: true}).Wait()
		if err != nil {
			return fmt.Errorf("status: %w", err)
		}

		currentCount := *status.NumMessages
		if currentCount > lastKnownCount {
			startSeq := lastKnownCount + 1
			endSeq := currentCount

			var seqSet imap.SeqSet
			seqSet.AddRange(startSeq, endSeq)

			headerSection := &imap.FetchItemBodySection{
				Specifier:    imap.PartSpecifierHeader,
				HeaderFields: []string{"From", "X-AnonAddy-Original-Sender", "To", "Delivered-To", "Subject", "Message-Id", "Date"},
				Peek:         true,
			}
			fetchOpts := &imap.FetchOptions{
				BodySection: []*imap.FetchItemBodySection{headerSection},
			}

			messages, err := client.Fetch(seqSet, fetchOpts).Collect()
			if err != nil {
				fmt.Printf("warn: fetch new messages: %v\n", err)
			} else {
				for _, msg := range messages {
					var raw []byte
					for _, v := range msg.BodySection {
						raw = v
						break
					}
					if len(raw) == 0 {
						continue
					}
					headers := make(map[string][]string)
					parseHeadersInto(raw, headers)
					processNewMessage(ctx, opts, headers)
				}
			}
			lastKnownCount = currentCount
		}
	}
}

func processNewMessage(ctx context.Context, opts MonitorOptions, headers map[string][]string) {
	aliasEmail := ExtractAliasEmail(headers, opts.AliasDomain)
	if aliasEmail == "" {
		return
	}

	senderEmail := ExtractSenderEmail(headers)
	if senderEmail == "" {
		return
	}
	senderDomain := DomainFromEmail(senderEmail)

	subject := firstHeader(headers, "Subject")
	messageID := firstHeader(headers, "Message-Id")

	now := time.Now()

	// Look up known senders for this alias
	knownSenders, err := opts.Store.KnownSendersForAlias(aliasEmail)
	if err != nil {
		fmt.Printf("warn: lookup senders for %s: %v\n", aliasEmail, err)
		return
	}
	knownDomains, err := opts.Store.KnownDomainsForAlias(aliasEmail)
	if err != nil {
		fmt.Printf("warn: lookup domains for %s: %v\n", aliasEmail, err)
		return
	}

	found, flagged := monitor.IsKnownSender(knownSenders, knownDomains, senderEmail, senderDomain)

	// Get account name for the notification
	accounts, _ := opts.Store.AccountsForAlias(aliasEmail)
	account := strings.Join(accounts, ", ")
	if account == "" {
		account = "UNMAPPED"
	}

	// Record the email
	wasNew := !found
	emailRecord := db.Email{
		AliasEmail: aliasEmail,
		FromAddr:   senderEmail,
		Subject:    subject,
		ReceivedAt: now,
		MessageID:  messageID,
		WasNew:     wasNew,
		Flagged:    false,
	}
	opts.Store.InsertEmail(emailRecord)

	if found && !flagged {
		// Known, not flagged → update last_seen silently
		opts.Store.UpsertKnownSender(db.KnownSender{
			AliasEmail:   aliasEmail,
			SenderEmail:  senderEmail,
			SenderDomain: senderDomain,
			FirstSeen:    now,
			LastSeen:     now,
		})
		return
	}

	// Determine domain context
	domainCount, _ := opts.Store.CountKnownSendersByDomain(aliasEmail, senderDomain)
	var domainContext string
	if domainCount > 0 {
		domainContext = fmt.Sprintf("known — %d address(es) seen", domainCount)
	} else {
		domainContext = "new domain"
	}

	// Send notification
	kind := "new"
	if flagged {
		kind = "flagged"
		fmt.Printf("flagged: %s → %s\n", aliasEmail, senderEmail)
	}

	n := notify.Notification{
		Kind:          kind,
		Alias:         aliasEmail,
		Account:       account,
		Sender:        senderEmail,
		Subject:       subject,
		Domain:        senderDomain,
		DomainContext: domainContext,
	}
	if err := opts.Notifier.Send(n); err != nil {
		fmt.Printf("warn: send notification: %v\n", err)
	}

	if !found {
		fmt.Printf("new:     %s → %s\n", aliasEmail, senderEmail)
		// Auto-learn the new sender
		opts.Store.UpsertKnownSender(db.KnownSender{
			AliasEmail:   aliasEmail,
			SenderEmail:  senderEmail,
			SenderDomain: senderDomain,
			FirstSeen:    now,
			LastSeen:     now,
		})
	}
}

func firstHeader(headers map[string][]string, key string) string {
	if vals, ok := headers[key]; ok && len(vals) > 0 {
		return vals[0]
	}
	return ""
}

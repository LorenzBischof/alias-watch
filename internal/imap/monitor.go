package imap

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	imap "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/lorenzbischof/alias-watch/internal/config"
	"github.com/lorenzbischof/alias-watch/internal/db"
	"github.com/lorenzbischof/alias-watch/internal/monitor"
	"github.com/lorenzbischof/alias-watch/internal/notify"
)

const idleInterval = 25 * time.Minute
const autoAddedAliasTitle = "AUTO-ADDED"

// MonitorOptions holds configuration for the IDLE monitor.
type MonitorOptions struct {
	IMAPConfig config.IMAPConfig
	Store      *db.Store
	Notifier   *notify.Client
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

type idleCounter struct {
	baseline uint32
	wakeCh   chan struct{}
}

func newIdleCounter(initial uint32) *idleCounter {
	return &idleCounter{
		baseline: initial,
		wakeCh:   make(chan struct{}, 1),
	}
}

func (c *idleCounter) OnMailbox(data *imapclient.UnilateralDataMailbox) {
	if data == nil || data.NumMessages == nil {
		return
	}
	// Wake the IDLE loop, but keep the baseline unchanged.
	select {
	case c.wakeCh <- struct{}{}:
	default:
	}
}

func (c *idleCounter) Baseline() uint32 {
	return c.baseline
}

func (c *idleCounter) UpdateBaseline(value uint32) {
	c.baseline = value
}

func (c *idleCounter) WakeCh() <-chan struct{} {
	return c.wakeCh
}

func (c *idleCounter) ResetWake() {
	for {
		select {
		case <-c.wakeCh:
		default:
			return
		}
	}
}

func runIDLESession(ctx context.Context, opts MonitorOptions) error {
	counter := newIdleCounter(0)

	clientOpts := &imapclient.Options{
		UnilateralDataHandler: &imapclient.UnilateralDataHandler{
			Mailbox: counter.OnMailbox,
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

	selected, err := client.Select(opts.IMAPConfig.Folder, &imap.SelectOptions{ReadOnly: true}).Wait()
	if err != nil {
		return fmt.Errorf("select: %w", err)
	}
	counter.UpdateBaseline(selected.NumMessages)

	for {
		if ctx.Err() != nil {
			return nil
		}

		counter.ResetWake()

		// Enter IDLE
		idleCmd, err := client.Idle()
		if err != nil {
			return fmt.Errorf("idle: %w", err)
		}

		// Wait up to 25 minutes or until context cancels
		idleTimer := time.NewTimer(idleInterval)
		select {
		case <-idleTimer.C:
		case <-counter.WakeCh():
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
		if currentCount > counter.Baseline() {
			startSeq := counter.Baseline() + 1
			endSeq := currentCount
			fmt.Printf("debug: mailbox count increased baseline=%d current=%d fetching seq=%d:%d\n",
				counter.Baseline(), currentCount, startSeq, endSeq)

			var seqSet imap.SeqSet
			seqSet.AddRange(startSeq, endSeq)

			headerSection := &imap.FetchItemBodySection{
				Specifier: imap.PartSpecifierHeader,
				Peek:      true,
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
			counter.UpdateBaseline(currentCount)
		}
	}
}

func processNewMessage(ctx context.Context, opts MonitorOptions, headers map[string][]string) {
	subject := firstHeader(headers, "Subject")
	messageID := firstHeader(headers, "Message-Id")
	now := time.Now()

	aliasEmail := ExtractAliasEmail(headers)
	if aliasEmail == "" {
		fmt.Printf("debug: skipped message %s reason=no-alias-header message_id=%q subject=%q headers=%s\n",
			now.Format(time.RFC3339), messageID, subject, headerKeys(headers))
		return
	}

	senderEmail := ExtractSenderEmail(headers)
	if senderEmail == "" {
		fmt.Printf("debug: skipped message %s reason=no-sender-header alias=%s message_id=%q subject=%q headers=%s\n",
			now.Format(time.RFC3339), aliasEmail, messageID, subject, headerKeys(headers))
		return
	}
	senderDomain := DomainFromEmail(senderEmail)
	fmt.Printf("debug: incoming %s subject=%q\n", now.Format(time.RFC3339), subject)

	aliasExists, err := opts.Store.AliasExists(aliasEmail)
	if err != nil {
		fmt.Printf("warn: check alias existence for %s: %v\n", aliasEmail, err)
		return
	}
	aliasIsNew := !aliasExists
	if aliasIsNew {
		if err := opts.Store.UpsertAlias(db.Alias{
			Email:    aliasEmail,
			AddyID:   aliasEmail,
			Active:   true,
			Title:    autoAddedAliasTitle,
			SyncedAt: now,
		}); err != nil {
			fmt.Printf("warn: create alias %s: %v\n", aliasEmail, err)
			return
		}
		fmt.Printf("alias:   auto-added %s\n", aliasEmail)
	}

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
		AliasIsNew:    aliasIsNew,
		DomainContext: domainContext,
	}
	if err := opts.Notifier.Send(n); err != nil {
		fmt.Printf("warn: send notification: %v\n", err)
	} else if strings.TrimSpace(opts.Notifier.NtfyURL) != "" {
		fmt.Printf("debug: notification sent kind=%s alias=%s sender=%s\n", kind, aliasEmail, senderEmail)
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

func headerKeys(headers map[string][]string) string {
	if len(headers) == 0 {
		return "[]"
	}
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return "[" + strings.Join(keys, ",") + "]"
}

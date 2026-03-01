package imap

import (
	"fmt"
	"strings"
	"time"

	imap "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/lorenzbischof/alias-watch/internal/db"
)

// LearnOptions holds the parameters for the learn operation.
type LearnOptions struct {
	Client *imapclient.Client
	Store  *db.Store
	Debug  bool
}

// Learn scans IMAP history to populate the known_senders table.
// It iterates over all folders on the server.
// Returns the count of new sender records upserted.
func Learn(opts LearnOptions) (int, error) {
	mailboxes, err := opts.Client.List("", "*", nil).Collect()
	if err != nil {
		return 0, fmt.Errorf("list folders: %w", err)
	}

	aliases, err := opts.Store.AllAliases()
	if err != nil {
		return 0, fmt.Errorf("get aliases: %w", err)
	}

	if opts.Debug {
		fmt.Printf("debug: %d aliases in DB, %d folders\n", len(aliases), len(mailboxes))
	}

	total := 0
	for _, mbox := range mailboxes {
		if opts.Debug {
			fmt.Printf("debug: scanning folder %s\n", mbox.Mailbox)
		}
		if _, err := opts.Client.Select(mbox.Mailbox, &imap.SelectOptions{ReadOnly: true}).Wait(); err != nil {
			fmt.Printf("  warn: select folder %s: %v\n", mbox.Mailbox, err)
			continue
		}
		for _, alias := range aliases {
			n, err := learnAlias(opts, alias.Email)
			if err != nil {
				fmt.Printf("  warn: [%s] alias %s: %v\n", mbox.Mailbox, alias.Email, err)
				continue
			}
			total += n
		}
	}
	return total, nil
}

func learnAlias(opts LearnOptions, aliasEmail string) (int, error) {
	criteria := &imap.SearchCriteria{
		Or: [][2]imap.SearchCriteria{
			{
				{Header: []imap.SearchCriteriaHeaderField{{Key: "To", Value: aliasEmail}}},
				{Header: []imap.SearchCriteriaHeaderField{{Key: "Delivered-To", Value: aliasEmail}}},
			},
		},
	}

	searchData, err := opts.Client.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return 0, fmt.Errorf("search: %w", err)
	}

	if searchData == nil || len(searchData.AllUIDs()) == 0 {
		return 0, nil
	}

	uids := searchData.AllUIDs()
	uidSet := imap.UIDSetNum(uids...)

	headerSection := &imap.FetchItemBodySection{
		Specifier:    imap.PartSpecifierHeader,
		HeaderFields: []string{"From", "X-AnonAddy-Original-Sender", "To", "Delivered-To", "Date", "Message-Id"},
		Peek:         true,
	}

	fetchOpts := &imap.FetchOptions{
		BodySection: []*imap.FetchItemBodySection{headerSection},
	}

	messages, err := opts.Client.Fetch(uidSet, fetchOpts).Collect()
	if err != nil {
		return 0, fmt.Errorf("fetch: %w", err)
	}

	noSection, noSender := 0, 0
	var newSenders []string
	for _, msg := range messages {
		var raw []byte
		for _, v := range msg.BodySection {
			raw = v
			break
		}
		if len(raw) == 0 {
			noSection++
			continue
		}

		headers := make(map[string][]string)
		parseHeadersInto(raw, headers)

		senderEmail := ExtractSenderEmail(headers)
		if senderEmail == "" {
			noSender++
			continue
		}
		senderDomain := DomainFromEmail(senderEmail)

		now := time.Now()
		ks := db.KnownSender{
			AliasEmail:   aliasEmail,
			SenderEmail:  senderEmail,
			SenderDomain: senderDomain,
			FirstSeen:    now,
			LastSeen:     now,
		}
		isNew, err := opts.Store.UpsertKnownSender(ks)
		if err != nil {
			fmt.Printf("  warn: [%s] upsert sender %s: %v\n", aliasEmail, senderEmail, err)
		} else if isNew {
			newSenders = append(newSenders, senderEmail)
		}
	}

	count := len(newSenders)
	if count > 0 {
		fmt.Printf("%s → %s\n", aliasEmail, strings.Join(newSenders, ", "))
	}

	if opts.Debug {
		updated := len(messages) - noSection - noSender - count
		fmt.Printf("debug: [%s] messages=%d new=%d updated=%d no-section=%d no-sender=%d\n",
			aliasEmail, len(messages), count, updated, noSection, noSender)
	}

	return count, nil
}

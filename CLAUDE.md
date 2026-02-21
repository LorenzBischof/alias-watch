# email-monitor — developer notes

## Keeping this file up to date

**IMPORTANT:** Whenever a bug is found, a library behaviour is discovered to differ from what's documented here, or a non-obvious design decision is made, update this file immediately — do not wait to be asked. This file is the authoritative source of hard-won knowledge about this codebase.

## CLI

```
Monitor email aliases for unexpected senders

Usage:
  email-monitor [command]

Available Commands:
  flag        Flag email + sender as phishing
  learn       Scan IMAP history to populate known_senders
  monitor     IMAP IDLE daemon — alert on new/flagged senders
  report      Print alias→account table to stdout
  sync        Pull addy.io aliases + KeePass entries into DB
  tui         Interactive two-pane TUI for managing known senders
  validate    Cross-validate data sources, print issues

Flags:
  -c, --config string   path to config file (default "config.yaml")

Use "email-monitor [command] --help" for more information about a command.
```

## What this tool does

Each addy.io alias is used for exactly one real-world service (GitHub, Amazon, etc.). The tool detects phishing by learning which senders are normal for each alias, then alerting via ntfy.sh when an unexpected sender appears.

The three data sources are kept in sync:
- **addy.io** — the canonical list of aliases
- **KeePassXC** — maps each alias email to an account name (the service it was created for)
- **IMAP** — reveals which senders have historically written to each alias

## Core data model

```
aliases  ──< alias_accounts    (one alias → many KeePass accounts)
aliases  ──< known_senders     (one alias → many trusted senders)
aliases  ──< emails            (one alias → many received messages)
```

`known_senders` stores exact sender addresses. Alias-level domain matching is stored in `known_domains`:
- exact match: `known_senders.sender_email` matches incoming sender
- domain match: `known_domains.sender_domain` matches incoming sender domain

Use domain rules for aliases where exact-address matching is too strict.

## Alert logic (monitor command)

For each new IMAP message:

1. Extract alias from `To`/`Delivered-To`, sender from `X-AnonAddy-Original-Sender` → fallback `From`.
2. Look up `known_senders` and `known_domains` for that alias.
3. **Known + not flagged** → silent. Update `last_seen`.
4. **Known + flagged** → alert ("Flagged sender"). The sender was manually marked as phishing via `flag <id>`.
5. **Unknown** → alert ("New sender") with domain-familiarity context, then auto-insert into `known_senders`. No manual approval needed; user only acts if something is wrong.

Domain-familiarity context in the notification: count existing `known_senders` rows for that alias where `sender_domain` matches. > 0 means "known domain, new address" (less suspicious); 0 means "new domain" (more suspicious).

## Flagging workflow

`flag <email-id>` sets `flagged = 1` on both the `emails` row and the corresponding `known_senders` row. From that point on, any future email from that sender to that alias triggers a notification even though the sender is technically "known". This is the only manual action the user ever needs to take.

## Development workflow (TDD)

**Always write or update the test first, then write the implementation.**

1. Write a failing test that captures the desired behaviour.
2. Verify it fails for the right reason (`go test` shows the expected error).
3. Write the minimal implementation to make it pass.
4. Run tests again to confirm green.

Never write implementation code without a corresponding test already in place. If a function signature needs to change, update the test to reflect the new signature before touching the implementation.

## Building

Always build with `CGO_ENABLED=0`. The `modernc.org/sqlite` driver is pure Go (no libsqlite3 needed), but the Go toolchain still tries to link CGO unless explicitly disabled:

```
CGO_ENABLED=0 go build ./cmd/email-monitor
CGO_ENABLED=0 go test ./...
```

## go-imap/v2 FetchMessageBuffer.BodySection

`FetchMessageBuffer.BodySection` is `map[*imap.FetchItemBodySection][]byte` — keyed by a **freshly-allocated pointer parsed from the IMAP response** (`readSectionSpec`), not the pointer you put in `FetchOptions.BodySection`. Direct lookup by the request pointer always misses. Iterate over the map instead:

```go
messages, _ := client.Fetch(set, &imap.FetchOptions{BodySection: []*imap.FetchItemBodySection{headerSection}}).Collect()
var raw []byte
for _, v := range messages[0].BodySection {
    raw = v
    break
}
```

This is safe as long as only one body section is requested per fetch call.

## Header key canonicalization

`parseHeadersInto` (in `internal/imap/headers.go`) normalises header keys to HTTP title-case: each word after a hyphen is capitalised, rest lowercased. The addy.io header `X-AnonAddy-Original-Sender` is therefore stored as `"X-Anonaddy-Original-Sender"`. Use that exact string when looking up the map.

## KeePass → alias mapping

Only KeePass entries whose **UserName** field contains `config.addyio.alias_domain` are imported. Entries without a matching username are silently ignored. The account name priority is: `Title` → URL stripped of `https://` prefix and trailing slash → `"<unnamed>"`.

## UnilateralDataHandler bug in monitor

`runIDLESession` registers a `UnilateralDataHandler.Mailbox` callback that overwrites `lastKnownCount` whenever the server pushes an EXISTS update during IDLE. After IDLE ends the code polls STATUS and compares `currentCount > lastKnownCount`. If the handler already updated `lastKnownCount` to the new value (which it will for any message that arrives while IDLE is active), the comparison is false and those messages are **not fetched**. Only messages that arrive in the window between IDLE closing and the STATUS response will be caught reliably. Fix: snapshot the count before entering IDLE and do not update it from the handler.

## TUI subcommand

`tui` opens a two-pane keyboard-driven interface:
- Left pane: all aliases (Account | Alias Email | Active)
- Right pane: known senders for the selected alias

Key bindings: `Tab` switch pane, `↑/k` `↓/j` move, `a` add sender, `d` delete, `f` toggle flagged, `e` toggle domain rule for the selected sender domain, `q`/`Ctrl+C` quit.

The add-sender form validates email format. Tab/Shift+Tab moves between form fields; Enter submits; Esc cancels.

After add or delete, `senderTable.GotoTop()` must be called before `SetRows` to avoid out-of-bounds cursor.

## learn: re-run behaviour

Running `learn` multiple times on the same mailbox is safe but redundant for aliases that already have known senders. On conflict, `UpsertKnownSender` only updates `last_seen`; no counters are inflated.

## Command dependency order

`validate` and `report` read the local DB only; they do not contact addy.io or the IMAP server. The expected first-run sequence is:

```
sync    # populates aliases + alias_accounts
learn   # populates known_senders + emails
validate / report / monitor
```

Running `validate` before `sync` will report every alias as having issues.

## Database driver registration

`modernc.org/sqlite` registers itself under the driver name `"sqlite"` (not `"sqlite3"`). The blank import in `internal/db/store.go` handles this. Do not replace with `mattn/go-sqlite3` — that requires CGO.

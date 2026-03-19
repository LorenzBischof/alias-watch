package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Store wraps a SQLite database and provides typed operations.
type Store struct {
	db *sql.DB
}

// Alias represents a row in the aliases table.
type Alias struct {
	Email    string
	AddyID   string
	Active   bool
	Title    string
	SyncedAt time.Time
}

// KnownSender represents a row in the known_senders table.
type KnownSender struct {
	ID           int64
	AliasEmail   string
	SenderEmail  string
	SenderDomain string
	Flagged      bool
	FirstSeen    time.Time
	LastSeen     time.Time
}

// KnownDomain represents an alias-level domain matching rule.
type KnownDomain struct {
	AliasEmail   string
	SenderDomain string
	Enabled      bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Email represents a row in the emails table.
type Email struct {
	ID         int64
	AliasEmail string
	FromAddr   string
	Subject    string
	ReceivedAt time.Time
	MessageID  string
	WasNew     bool
	Flagged    bool
}

// Open opens (or creates) the SQLite database at path and applies schema migrations.
func Open(path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}
	sqldb, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	sqldb.SetMaxOpenConns(1) // SQLite doesn't support concurrent writes
	if _, err := sqldb.Exec(schema); err != nil {
		sqldb.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	if err := applyMigrations(sqldb); err != nil {
		sqldb.Close()
		return nil, fmt.Errorf("apply migrations: %w", err)
	}
	return &Store{db: sqldb}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// UpsertAlias inserts or updates an alias row.
// On conflict, title is only written if the existing value is empty,
// so locally-edited titles survive sync.
func (s *Store) UpsertAlias(a Alias) error {
	_, err := s.db.Exec(`
		INSERT INTO aliases (email, addy_id, active, title, synced_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(email) DO UPDATE SET
			addy_id   = excluded.addy_id,
			active    = excluded.active,
			title     = CASE WHEN aliases.title IS NULL OR aliases.title = ''
			                 THEN excluded.title
			                 ELSE aliases.title END,
			synced_at = excluded.synced_at`,
		a.Email, a.AddyID, boolToInt(a.Active), a.Title, a.SyncedAt.UTC().Format(time.RFC3339),
	)
	return err
}

// AliasExists returns whether an alias row exists for the given email.
func (s *Store) AliasExists(email string) (bool, error) {
	var exists int
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM aliases WHERE email = ?)`, email).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists == 1, nil
}

// AllAliases returns all aliases in the database.
func (s *Store) AllAliases() ([]Alias, error) {
	rows, err := s.db.Query(`SELECT email, addy_id, active, title, synced_at FROM aliases`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var aliases []Alias
	for rows.Next() {
		var a Alias
		var activeInt int
		var syncedAt string
		var title sql.NullString
		if err := rows.Scan(&a.Email, &a.AddyID, &activeInt, &title, &syncedAt); err != nil {
			return nil, err
		}
		a.Active = activeInt != 0
		a.Title = title.String
		a.SyncedAt, _ = time.Parse(time.RFC3339, syncedAt)
		aliases = append(aliases, a)
	}
	return aliases, rows.Err()
}

// AliasLastSeen returns the latest known_sender last_seen per alias email.
func (s *Store) AliasLastSeen() (map[string]time.Time, error) {
	rows, err := s.db.Query(`
		SELECT alias_email, MAX(last_seen)
		FROM known_senders
		GROUP BY alias_email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]time.Time)
	for rows.Next() {
		var aliasEmail string
		var raw string
		if err := rows.Scan(&aliasEmail, &raw); err != nil {
			return nil, err
		}
		ts, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return nil, err
		}
		out[aliasEmail] = ts
	}
	return out, rows.Err()
}

// ReplaceAliasAccounts replaces all accounts for an alias.
func (s *Store) ReplaceAliasAccounts(aliasEmail string, accounts []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM alias_accounts WHERE alias_email = ?`, aliasEmail); err != nil {
		return err
	}
	for _, acc := range accounts {
		if _, err := tx.Exec(`INSERT INTO alias_accounts (alias_email, account) VALUES (?, ?)`, aliasEmail, acc); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// AccountsForAlias returns all account names linked to an alias.
func (s *Store) AccountsForAlias(aliasEmail string) ([]string, error) {
	rows, err := s.db.Query(`SELECT account FROM alias_accounts WHERE alias_email = ? ORDER BY account`, aliasEmail)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var accounts []string
	for rows.Next() {
		var acc string
		if err := rows.Scan(&acc); err != nil {
			return nil, err
		}
		accounts = append(accounts, acc)
	}
	return accounts, rows.Err()
}

// AliasAccountPair is one row of the flat alias→account view.
type AliasAccountPair struct {
	AliasEmail string
	Account    string
	Title      string
	Active     bool
}

// AllAliasAccountPairs returns all alias→account rows (UNMAPPED if no accounts).
func (s *Store) AllAliasAccountPairs() ([]AliasAccountPair, error) {
	rows, err := s.db.Query(`
		SELECT a.email, COALESCE(aa.account, 'UNMAPPED'), a.title, a.active
		FROM aliases a
		LEFT JOIN alias_accounts aa ON a.email = aa.alias_email
		ORDER BY aa.account IS NULL ASC, a.email, aa.account`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pairs []AliasAccountPair
	for rows.Next() {
		var p AliasAccountPair
		var activeInt int
		if err := rows.Scan(&p.AliasEmail, &p.Account, &p.Title, &activeInt); err != nil {
			return nil, err
		}
		p.Active = activeInt != 0
		pairs = append(pairs, p)
	}
	return pairs, rows.Err()
}

// UpdateAliasTitle updates the title field for an alias.
func (s *Store) UpdateAliasTitle(email, title string) error {
	_, err := s.db.Exec(`UPDATE aliases SET title = ? WHERE email = ?`, title, email)
	return err
}

// UpsertKnownSender inserts or updates a known sender.
// Returns true if the sender was newly inserted, false if it already existed.
func (s *Store) UpsertKnownSender(ks KnownSender) (bool, error) {
	res, err := s.db.Exec(`
		INSERT OR IGNORE INTO known_senders (alias_email, sender_email, sender_domain, flagged, first_seen, last_seen)
		VALUES (?, ?, ?, ?, ?, ?)`,
		ks.AliasEmail, ks.SenderEmail, ks.SenderDomain,
		boolToInt(ks.Flagged),
		ks.FirstSeen.UTC().Format(time.RFC3339),
		ks.LastSeen.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n == 1 {
		return true, nil
	}
	_, err = s.db.Exec(`
		UPDATE known_senders SET last_seen = ?
		WHERE alias_email = ? AND sender_email = ?`,
		ks.LastSeen.UTC().Format(time.RFC3339),
		ks.AliasEmail, ks.SenderEmail,
	)
	return false, err
}

// KnownSendersForAlias returns all known senders for a given alias.
func (s *Store) KnownSendersForAlias(aliasEmail string) ([]KnownSender, error) {
	rows, err := s.db.Query(`
		SELECT id, alias_email, sender_email, sender_domain, flagged, first_seen, last_seen
		FROM known_senders WHERE alias_email = ?`, aliasEmail)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanKnownSenders(rows)
}

// UpsertKnownDomain creates or updates an alias-level domain rule.
func (s *Store) UpsertKnownDomain(kd KnownDomain) error {
	now := time.Now().UTC().Format(time.RFC3339)
	createdAt := kd.CreatedAt.UTC().Format(time.RFC3339)
	if kd.CreatedAt.IsZero() {
		createdAt = now
	}
	updatedAt := kd.UpdatedAt.UTC().Format(time.RFC3339)
	if kd.UpdatedAt.IsZero() {
		updatedAt = now
	}
	_, err := s.db.Exec(`
		INSERT INTO known_domains (alias_email, sender_domain, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(alias_email, sender_domain) DO UPDATE SET
			enabled = excluded.enabled,
			updated_at = excluded.updated_at`,
		kd.AliasEmail,
		kd.SenderDomain,
		boolToInt(kd.Enabled),
		createdAt,
		updatedAt,
	)
	return err
}

// DeleteKnownDomain removes an alias-level domain rule.
func (s *Store) DeleteKnownDomain(aliasEmail, senderDomain string) error {
	_, err := s.db.Exec(`DELETE FROM known_domains WHERE alias_email = ? AND sender_domain = ?`, aliasEmail, senderDomain)
	return err
}

// KnownDomainsForAlias returns all domain rules for an alias.
func (s *Store) KnownDomainsForAlias(aliasEmail string) ([]KnownDomain, error) {
	rows, err := s.db.Query(`
		SELECT alias_email, sender_domain, enabled, created_at, updated_at
		FROM known_domains
		WHERE alias_email = ?
		ORDER BY sender_domain`, aliasEmail)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []KnownDomain
	for rows.Next() {
		var kd KnownDomain
		var enabledInt int
		var createdAt, updatedAt string
		if err := rows.Scan(&kd.AliasEmail, &kd.SenderDomain, &enabledInt, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		kd.Enabled = enabledInt != 0
		kd.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		kd.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		out = append(out, kd)
	}
	return out, rows.Err()
}

// CountKnownSendersByDomain returns the number of known senders for an alias with a given domain.
func (s *Store) CountKnownSendersByDomain(aliasEmail, domain string) (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM known_senders
		WHERE alias_email = ? AND sender_domain = ?`, aliasEmail, domain).Scan(&count)
	return count, err
}

// InsertEmail records a received email.
func (s *Store) InsertEmail(e Email) (int64, error) {
	var msgID interface{}
	if e.MessageID != "" {
		msgID = e.MessageID
	}
	res, err := s.db.Exec(`
		INSERT OR IGNORE INTO emails (alias_email, from_addr, subject, received_at, message_id, was_new, flagged)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.AliasEmail, e.FromAddr, e.Subject,
		e.ReceivedAt.UTC().Format(time.RFC3339),
		msgID,
		boolToInt(e.WasNew), boolToInt(e.Flagged),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// FlagEmail sets flagged=1 on the email and the matching known_sender.
func (s *Store) FlagEmail(emailID int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var aliasEmail, fromAddr string
	if err := tx.QueryRow(`SELECT alias_email, from_addr FROM emails WHERE id = ?`, emailID).
		Scan(&aliasEmail, &fromAddr); err != nil {
		return fmt.Errorf("email %d not found: %w", emailID, err)
	}
	if _, err := tx.Exec(`UPDATE emails SET flagged = 1 WHERE id = ?`, emailID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE known_senders SET flagged = 1 WHERE alias_email = ? AND sender_email = ?`,
		aliasEmail, fromAddr); err != nil {
		return err
	}
	return tx.Commit()
}

// EmailsForAlias returns all emails for an alias.
func (s *Store) EmailsForAlias(aliasEmail string) ([]Email, error) {
	rows, err := s.db.Query(`
		SELECT id, alias_email, from_addr, subject, received_at, COALESCE(message_id,''), was_new, flagged
		FROM emails WHERE alias_email = ? ORDER BY received_at DESC`, aliasEmail)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEmails(rows)
}

// AllKnownSenders returns all known_senders rows ordered by alias then sender email.
func (s *Store) AllKnownSenders() ([]KnownSender, error) {
	rows, err := s.db.Query(`
		SELECT id, alias_email, sender_email, sender_domain, flagged, first_seen, last_seen
		FROM known_senders ORDER BY alias_email, sender_email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanKnownSenders(rows)
}

// KnownSenderCountForAlias returns the count of known senders for an alias.
func (s *Store) KnownSenderCountForAlias(aliasEmail string) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM known_senders WHERE alias_email = ?`, aliasEmail).Scan(&count)
	return count, err
}

// EmailCountForAlias returns the count of emails for an alias.
func (s *Store) EmailCountForAlias(aliasEmail string) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM emails WHERE alias_email = ?`, aliasEmail).Scan(&count)
	return count, err
}

// MostUsedSenderForAlias returns the most frequent from_addr for an alias from
// historical emails, including how many times it appears.
func (s *Store) MostUsedSenderForAlias(aliasEmail string) (string, int, error) {
	var sender string
	var count int
	err := s.db.QueryRow(`
		SELECT from_addr, COUNT(*) AS c
		FROM emails
		WHERE alias_email = ?
		GROUP BY from_addr
		ORDER BY c DESC, from_addr ASC
		LIMIT 1`, aliasEmail).Scan(&sender, &count)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", 0, nil
		}
		return "", 0, err
	}
	return sender, count, nil
}

// MostUsedDomainForAlias returns the most frequent sender domain for an alias
// from historical emails, including how many times it appears.
func (s *Store) MostUsedDomainForAlias(aliasEmail string) (string, int, error) {
	var domain string
	var count int
	err := s.db.QueryRow(`
		SELECT LOWER(SUBSTR(from_addr, INSTR(from_addr, '@') + 1)) AS domain, COUNT(*) AS c
		FROM emails
		WHERE alias_email = ? AND INSTR(from_addr, '@') > 1
		GROUP BY domain
		ORDER BY c DESC, domain ASC
		LIMIT 1`, aliasEmail).Scan(&domain, &count)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", 0, nil
		}
		return "", 0, err
	}
	return domain, count, nil
}

func scanKnownSenders(rows *sql.Rows) ([]KnownSender, error) {
	var senders []KnownSender
	for rows.Next() {
		var ks KnownSender
		var flaggedInt int
		var firstSeen, lastSeen string
		if err := rows.Scan(&ks.ID, &ks.AliasEmail, &ks.SenderEmail, &ks.SenderDomain,
			&flaggedInt, &firstSeen, &lastSeen); err != nil {
			return nil, err
		}
		ks.Flagged = flaggedInt != 0
		ks.FirstSeen, _ = time.Parse(time.RFC3339, firstSeen)
		ks.LastSeen, _ = time.Parse(time.RFC3339, lastSeen)
		senders = append(senders, ks)
	}
	return senders, rows.Err()
}

func scanEmails(rows *sql.Rows) ([]Email, error) {
	var emails []Email
	for rows.Next() {
		var e Email
		var wasNewInt, flaggedInt int
		var receivedAt string
		if err := rows.Scan(&e.ID, &e.AliasEmail, &e.FromAddr, &e.Subject,
			&receivedAt, &e.MessageID, &wasNewInt, &flaggedInt); err != nil {
			return nil, err
		}
		e.WasNew = wasNewInt != 0
		e.Flagged = flaggedInt != 0
		e.ReceivedAt, _ = time.Parse(time.RFC3339, receivedAt)
		emails = append(emails, e)
	}
	return emails, rows.Err()
}

// DeleteKnownSender removes a known sender by its ID.
func (s *Store) DeleteKnownSender(id int64) error {
	_, err := s.db.Exec(`DELETE FROM known_senders WHERE id = ?`, id)
	return err
}

// UpdateKnownSender updates sender_email, sender_domain, flagged, and last_seen for an existing known sender.
func (s *Store) UpdateKnownSender(ks KnownSender) error {
	_, err := s.db.Exec(`
		UPDATE known_senders
		SET sender_email = ?, sender_domain = ?, flagged = ?, last_seen = ?
		WHERE id = ?`,
		ks.SenderEmail, ks.SenderDomain,
		boolToInt(ks.Flagged),
		ks.LastSeen.UTC().Format(time.RFC3339),
		ks.ID,
	)
	return err
}

func applyMigrations(sqldb *sql.DB) error {
	// Rename aliases.description → aliases.title.
	hasDescription, err := hasColumn(sqldb, "aliases", "description")
	if err != nil {
		return err
	}
	if hasDescription {
		if _, err := sqldb.Exec(`ALTER TABLE aliases RENAME COLUMN description TO title`); err != nil {
			return fmt.Errorf("rename description to title: %w", err)
		}
	}

	// Backfill alias-level domain rules from legacy match_type=domain rows.
	hasMatchType, err := hasColumn(sqldb, "known_senders", "match_type")
	if err != nil {
		return err
	}
	if !hasMatchType {
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = sqldb.Exec(`
		INSERT OR IGNORE INTO known_domains (alias_email, sender_domain, enabled, created_at, updated_at)
		SELECT DISTINCT alias_email, sender_domain, 1, ?, ?
		FROM known_senders
		WHERE match_type = 'domain'`,
		now, now,
	)
	return err
}

func hasColumn(sqldb *sql.DB, tableName, columnName string) (bool, error) {
	rows, err := sqldb.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, tableName))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var ctype string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == columnName {
			return true, nil
		}
	}
	return false, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

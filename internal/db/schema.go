package db

const schema = `
CREATE TABLE IF NOT EXISTS aliases (
    email       TEXT PRIMARY KEY,
    addy_id     TEXT NOT NULL,
    active      INTEGER NOT NULL DEFAULT 1,
    title TEXT,
    synced_at   TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS alias_accounts (
    alias_email  TEXT NOT NULL,
    account      TEXT NOT NULL,
    PRIMARY KEY (alias_email, account)
);

CREATE TABLE IF NOT EXISTS known_senders (
    id            INTEGER PRIMARY KEY,
    alias_email   TEXT NOT NULL,
    sender_email  TEXT NOT NULL,
    sender_domain TEXT NOT NULL,
    flagged       INTEGER NOT NULL DEFAULT 0,
    first_seen    TEXT NOT NULL,
    last_seen     TEXT NOT NULL,
    UNIQUE (alias_email, sender_email)
);

CREATE TABLE IF NOT EXISTS known_domains (
    alias_email   TEXT NOT NULL,
    sender_domain TEXT NOT NULL,
    enabled       INTEGER NOT NULL DEFAULT 1,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    PRIMARY KEY (alias_email, sender_domain)
);

CREATE TABLE IF NOT EXISTS emails (
    id           INTEGER PRIMARY KEY,
    alias_email  TEXT NOT NULL,
    from_addr    TEXT NOT NULL,
    subject      TEXT,
    received_at  TEXT NOT NULL,
    message_id   TEXT UNIQUE,
    was_new      INTEGER NOT NULL DEFAULT 0,
    flagged      INTEGER NOT NULL DEFAULT 0
);
`

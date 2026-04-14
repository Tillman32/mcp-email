package cache

// Schema contains SQL schema definitions for the cache
const Schema = `
-- Accounts table
CREATE TABLE IF NOT EXISTS accounts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    imap_host TEXT NOT NULL,
    imap_port INTEGER NOT NULL,
    imap_username TEXT NOT NULL,
    smtp_host TEXT NOT NULL,
    smtp_port INTEGER NOT NULL,
    smtp_username TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Folders table
CREATE TABLE IF NOT EXISTS folders (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    path TEXT NOT NULL,
    message_count INTEGER DEFAULT 0,
    last_synced DATETIME,
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE,
    UNIQUE(account_id, path)
);

-- Emails table
CREATE TABLE IF NOT EXISTS emails (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL,
    folder_id INTEGER NOT NULL,
    uid INTEGER NOT NULL,
    message_id TEXT NOT NULL,
    subject TEXT,
    sender_name TEXT,
    sender_email TEXT,
    recipients TEXT,
    date DATETIME NOT NULL,
    body_text TEXT,
    body_html TEXT,
    headers TEXT,
    flags TEXT,
    cached_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE,
    FOREIGN KEY (folder_id) REFERENCES folders(id) ON DELETE CASCADE,
    UNIQUE(account_id, folder_id, uid)
);

-- Create indexes for faster queries
CREATE INDEX IF NOT EXISTS idx_emails_account_id ON emails(account_id);
CREATE INDEX IF NOT EXISTS idx_emails_folder_id ON emails(folder_id);
CREATE INDEX IF NOT EXISTS idx_emails_date ON emails(date);
CREATE INDEX IF NOT EXISTS idx_emails_sender_email ON emails(sender_email);
CREATE INDEX IF NOT EXISTS idx_emails_message_id ON emails(message_id);
CREATE INDEX IF NOT EXISTS idx_folders_account_id ON folders(account_id);

-- Full-text search index
CREATE VIRTUAL TABLE IF NOT EXISTS emails_fts USING fts5(
    subject,
    sender_email,
    sender_name,
    body_text,
    content='emails',
    content_rowid='id'
);

-- Triggers for FTS
CREATE TRIGGER IF NOT EXISTS emails_fts_insert AFTER INSERT ON emails BEGIN
    INSERT INTO emails_fts(rowid, subject, sender_email, sender_name, body_text)
    VALUES (new.id, new.subject, new.sender_email, new.sender_name, new.body_text);
END;

CREATE TRIGGER IF NOT EXISTS emails_fts_update AFTER UPDATE ON emails BEGIN
    UPDATE emails_fts SET
        subject = new.subject,
        sender_email = new.sender_email,
        sender_name = new.sender_name,
        body_text = new.body_text
    WHERE rowid = new.id;
END;

CREATE TRIGGER IF NOT EXISTS emails_fts_delete AFTER DELETE ON emails BEGIN
    DELETE FROM emails_fts WHERE rowid = old.id;
END;

-- Unsubscribe link cache
CREATE TABLE IF NOT EXISTS unsubscribe_links (
    email_id INTEGER PRIMARY KEY,
    list_unsubscribe TEXT,
    body_links      TEXT,
    one_click       TEXT,
    cached_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (email_id) REFERENCES emails(id) ON DELETE CASCADE
);
`

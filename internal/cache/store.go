package cache

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/Tillman32/mcp-email/internal/config"
	"github.com/Tillman32/mcp-email/pkg/types"
)

// Store provides methods for storing and retrieving data from the cache
type Store struct {
	cache  *Cache
	logger *logrus.Logger
}

// NewStore creates a new store instance
func NewStore(cache *Cache, logger *logrus.Logger) *Store {
	return &Store{
		cache:  cache,
		logger: logger,
	}
}

// UpsertAccount upserts an account in the cache
func (s *Store) UpsertAccount(acc *config.AccountConfig) (int, error) {
	query := `
		INSERT INTO accounts (name, imap_host, imap_port, imap_username, smtp_host, smtp_port, smtp_username, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(name) DO UPDATE SET
			imap_host = excluded.imap_host,
			imap_port = excluded.imap_port,
			imap_username = excluded.imap_username,
			smtp_host = excluded.smtp_host,
			smtp_port = excluded.smtp_port,
			smtp_username = excluded.smtp_username,
			updated_at = CURRENT_TIMESTAMP
	`
	result, err := s.cache.DB().Exec(query, acc.Name, acc.IMAPHost, acc.IMAPPort, acc.IMAPUsername, acc.SMTPHost, acc.SMTPPort, acc.SMTPUsername)
	if err != nil {
		return 0, fmt.Errorf("failed to upsert account: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		// If insert failed, try to get existing ID
		var accountID int
		err := s.cache.DB().QueryRow("SELECT id FROM accounts WHERE name = ?", acc.Name).Scan(&accountID)
		if err != nil {
			return 0, fmt.Errorf("failed to get account ID: %w", err)
		}
		return accountID, nil
	}

	return int(id), nil
}

// GetAccountID returns the account ID by name
func (s *Store) GetAccountID(name string) (int, error) {
	var id int
	err := s.cache.DB().QueryRow("SELECT id FROM accounts WHERE name = ?", name).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("account not found: %s", name)
	}
	return id, nil
}

// UpsertFolder upserts a folder in the cache
func (s *Store) UpsertFolder(accountID int, name, path string, messageCount int) (int, error) {
	query := `
		INSERT INTO folders (account_id, name, path, message_count, last_synced)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(account_id, path) DO UPDATE SET
			name = excluded.name,
			message_count = excluded.message_count,
			last_synced = CURRENT_TIMESTAMP
	`
	result, err := s.cache.DB().Exec(query, accountID, name, path, messageCount)
	if err != nil {
		return 0, fmt.Errorf("failed to upsert folder: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		// Get existing ID
		var folderID int
		err := s.cache.DB().QueryRow("SELECT id FROM folders WHERE account_id = ? AND path = ?", accountID, path).Scan(&folderID)
		if err != nil {
			return 0, fmt.Errorf("failed to get folder ID: %w", err)
		}
		return folderID, nil
	}

	return int(id), nil
}

// UpsertEmail upserts an email in the cache
func (s *Store) UpsertEmail(email *types.Email) error {
	// Serialize recipients, headers, and flags
	recipientsJSON, err := json.Marshal(email.Recipients)
	if err != nil {
		return fmt.Errorf("failed to marshal recipients: %w", err)
	}
	headersJSON, err := json.Marshal(email.Headers)
	if err != nil {
		return fmt.Errorf("failed to marshal headers: %w", err)
	}
	flagsJSON, err := json.Marshal(email.Flags)
	if err != nil {
		return fmt.Errorf("failed to marshal flags: %w", err)
	}

	query := `
		INSERT INTO emails (account_id, folder_id, uid, message_id, subject, sender_name, sender_email, recipients, date, body_text, body_html, headers, flags)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(account_id, folder_id, uid) DO UPDATE SET
			message_id = excluded.message_id,
			subject = excluded.subject,
			sender_name = excluded.sender_name,
			sender_email = excluded.sender_email,
			recipients = excluded.recipients,
			date = excluded.date,
			body_text = excluded.body_text,
			body_html = excluded.body_html,
			headers = excluded.headers,
			flags = excluded.flags,
			cached_at = CURRENT_TIMESTAMP
	`
	_, err = s.cache.DB().Exec(query,
		email.AccountID,
		email.FolderID,
		email.UID,
		email.MessageID,
		email.Subject,
		email.SenderName,
		email.SenderEmail,
		string(recipientsJSON),
		email.Date,
		email.BodyText,
		email.BodyHTML,
		string(headersJSON),
		string(flagsJSON),
	)
	if err != nil {
		return fmt.Errorf("failed to upsert email: %w", err)
	}

	return nil
}

// GetEmail retrieves an email by ID
func (s *Store) GetEmail(emailID int64) (*types.Email, error) {
	query := `
		SELECT e.id, e.account_id, a.name, e.folder_id, f.path, e.uid, e.message_id, e.subject, e.sender_name, e.sender_email, e.recipients, e.date, e.body_text, e.body_html, e.headers, e.flags, e.cached_at
		FROM emails e
		JOIN accounts a ON e.account_id = a.id
		JOIN folders f ON e.folder_id = f.id
		WHERE e.id = ?
	`
	var email types.Email
	var recipientsJSON, headersJSON, flagsJSON string
	var dateStr string

	err := s.cache.DB().QueryRow(query, emailID).Scan(
		&email.ID,
		&email.AccountID,
		&email.AccountName,
		&email.FolderID,
		&email.FolderPath,
		&email.UID,
		&email.MessageID,
		&email.Subject,
		&email.SenderName,
		&email.SenderEmail,
		&recipientsJSON,
		&dateStr,
		&email.BodyText,
		&email.BodyHTML,
		&headersJSON,
		&flagsJSON,
		&email.CachedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("email not found: %d", emailID)
		}
		return nil, fmt.Errorf("failed to get email: %w", err)
	}

	// Parse date
	email.Date, err = time.Parse(time.RFC3339, dateStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse date: %w", err)
	}

	// Deserialize JSON fields
	if err := json.Unmarshal([]byte(recipientsJSON), &email.Recipients); err != nil {
		return nil, fmt.Errorf("failed to unmarshal recipients: %w", err)
	}
	if err := json.Unmarshal([]byte(headersJSON), &email.Headers); err != nil {
		return nil, fmt.Errorf("failed to unmarshal headers: %w", err)
	}
	if err := json.Unmarshal([]byte(flagsJSON), &email.Flags); err != nil {
		return nil, fmt.Errorf("failed to unmarshal flags: %w", err)
	}

	return &email, nil
}

// ListFolders lists folders for an account
func (s *Store) ListFolders(accountID *int) ([]types.Folder, error) {
	var query string
	var args []interface{}

	if accountID != nil {
		query = `
			SELECT f.id, f.account_id, a.name, f.name, f.path, f.message_count, f.last_synced
			FROM folders f
			JOIN accounts a ON f.account_id = a.id
			WHERE f.account_id = ?
			ORDER BY f.path
		`
		args = []interface{}{*accountID}
	} else {
		query = `
			SELECT f.id, f.account_id, a.name, f.name, f.path, f.message_count, f.last_synced
			FROM folders f
			JOIN accounts a ON f.account_id = a.id
			ORDER BY a.name, f.path
		`
	}

	rows, err := s.cache.DB().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query folders: %w", err)
	}
	defer rows.Close()

	var folders []types.Folder
	for rows.Next() {
		var folder types.Folder
		var lastSynced sql.NullString

		err := rows.Scan(
			&folder.ID,
			&folder.AccountID,
			&folder.AccountName,
			&folder.Name,
			&folder.Path,
			&folder.MessageCount,
			&lastSynced,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan folder: %w", err)
		}

		if lastSynced.Valid {
			t, err := time.Parse(time.RFC3339, lastSynced.String)
			if err == nil {
				folder.LastSynced = &t
			}
		}

		folders = append(folders, folder)
	}

	return folders, nil
}

// HasEmails checks if an account has any cached emails
func (s *Store) HasEmails(accountID int) (bool, error) {
	var count int
	err := s.cache.DB().QueryRow("SELECT COUNT(*) FROM emails WHERE account_id = ?", accountID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check emails count: %w", err)
	}
	return count > 0, nil
}

// HasAnyEmails checks if there are any cached emails
func (s *Store) HasAnyEmails() (bool, error) {
	var count int
	err := s.cache.DB().QueryRow("SELECT COUNT(*) FROM emails").Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check emails count: %w", err)
	}
	return count > 0, nil
}

package cache

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Tillman32/mcp-email/pkg/types"
)

// SearchOptions contains search parameters
type SearchOptions struct {
	AccountID *int
	FolderID  *int
	Sender    *string
	Recipient *string
	Subject   *string
	Body      *string
	DateFrom  *time.Time
	DateTo    *time.Time
	Limit     int
}

// Search performs a search on cached emails
func (s *Store) Search(opts SearchOptions) ([]types.EmailSummary, error) {
	var conditions []string
	var args []interface{}

	// Build WHERE clause
	if opts.AccountID != nil {
		conditions = append(conditions, "e.account_id = ?")
		args = append(args, *opts.AccountID)
	}

	if opts.FolderID != nil {
		conditions = append(conditions, "e.folder_id = ?")
		args = append(args, *opts.FolderID)
	}

	if opts.Sender != nil {
		conditions = append(conditions, "(e.sender_email LIKE ? OR e.sender_name LIKE ?)")
		searchTerm := "%" + *opts.Sender + "%"
		args = append(args, searchTerm, searchTerm)
	}

	if opts.Recipient != nil {
		conditions = append(conditions, "e.recipients LIKE ?")
		args = append(args, "%"+*opts.Recipient+"%")
	}

	if opts.Subject != nil {
		conditions = append(conditions, "e.subject LIKE ?")
		args = append(args, "%"+*opts.Subject+"%")
	}

	if opts.DateFrom != nil {
		conditions = append(conditions, "e.date >= ?")
		args = append(args, opts.DateFrom)
	}

	if opts.DateTo != nil {
		conditions = append(conditions, "e.date <= ?")
		args = append(args, opts.DateTo)
	}

	// Full-text search on body
	if opts.Body != nil {
		// Use FTS5 for body search
		conditions = append(conditions, "e.id IN (SELECT rowid FROM emails_fts WHERE emails_fts MATCH ?)")
		// Escape special characters for FTS5
		bodyQuery := strings.ReplaceAll(*opts.Body, "\"", "\"\"")
		bodyQuery = strings.ReplaceAll(bodyQuery, "'", "''")
		args = append(args, bodyQuery)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Set default limit
	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	query := fmt.Sprintf(`
		SELECT e.id, a.name, f.path, e.subject, e.sender_name, e.sender_email, e.date, e.body_text
		FROM emails e
		JOIN accounts a ON e.account_id = a.id
		JOIN folders f ON e.folder_id = f.id
		%s
		ORDER BY e.date DESC
		LIMIT ?
	`, whereClause)

	args = append(args, limit)

	rows, err := s.cache.DB().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search emails: %w", err)
	}
	defer rows.Close()

	var results []types.EmailSummary
	for rows.Next() {
		var summary types.EmailSummary
		var dateStr string
		var bodyText sql.NullString

		err := rows.Scan(
			&summary.ID,
			&summary.AccountName,
			&summary.FolderPath,
			&summary.Subject,
			&summary.SenderName,
			&summary.SenderEmail,
			&dateStr,
			&bodyText,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan email: %w", err)
		}

		// Parse date
		summary.Date, err = time.Parse("2006-01-02 15:04:05", dateStr)
		if err != nil {
			summary.Date, err = time.Parse(time.RFC3339, dateStr)
			if err != nil {
				summary.Date = time.Time{}
			}
		}

		// Create snippet from body
		if bodyText.Valid && len(bodyText.String) > 0 {
			snippet := bodyText.String
			if len(snippet) > 200 {
				snippet = snippet[:200] + "..."
			}
			summary.Snippet = snippet
		}

		results = append(results, summary)
	}

	return results, nil
}

// SearchFTS performs a full-text search using FTS5
func (s *Store) SearchFTS(query string, accountID *int, limit int) ([]types.EmailSummary, error) {
	var conditions []string
	var args []interface{}

	// Escape query for FTS5
	query = strings.ReplaceAll(query, "\"", "\"\"")
	query = strings.ReplaceAll(query, "'", "''")

	// FTS5 search
	conditions = append(conditions, "e.id IN (SELECT rowid FROM emails_fts WHERE emails_fts MATCH ?)")
	args = append(args, query)

	if accountID != nil {
		conditions = append(conditions, "e.account_id = ?")
		args = append(args, *accountID)
	}

	whereClause := "WHERE " + strings.Join(conditions, " AND ")

	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	sqlQuery := fmt.Sprintf(`
		SELECT e.id, a.name, f.path, e.subject, e.sender_name, e.sender_email, e.date, e.body_text
		FROM emails e
		JOIN accounts a ON e.account_id = a.id
		JOIN folders f ON e.folder_id = f.id
		%s
		ORDER BY e.date DESC
		LIMIT ?
	`, whereClause)

	args = append(args, limit)

	rows, err := s.cache.DB().Query(sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to perform FTS search: %w", err)
	}
	defer rows.Close()

	var results []types.EmailSummary
	for rows.Next() {
		var summary types.EmailSummary
		var dateStr string
		var bodyText sql.NullString

		err := rows.Scan(
			&summary.ID,
			&summary.AccountName,
			&summary.FolderPath,
			&summary.Subject,
			&summary.SenderName,
			&summary.SenderEmail,
			&dateStr,
			&bodyText,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan email: %w", err)
		}

		// Parse date
		summary.Date, err = time.Parse("2006-01-02 15:04:05", dateStr)
		if err != nil {
			summary.Date, err = time.Parse(time.RFC3339, dateStr)
			if err != nil {
				summary.Date = time.Time{}
			}
		}

		// Create snippet
		if bodyText.Valid && len(bodyText.String) > 0 {
			snippet := bodyText.String
			if len(snippet) > 200 {
				snippet = snippet[:200] + "..."
			}
			summary.Snippet = snippet
		}

		results = append(results, summary)
	}

	return results, nil
}

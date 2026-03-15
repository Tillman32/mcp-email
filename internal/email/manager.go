package email

import (
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/Tillman32/mcp-email/internal/cache"
	"github.com/Tillman32/mcp-email/internal/config"
)

// Manager manages email operations
type Manager struct {
	accountManager *AccountManager
	store          *cache.Store
	config         *config.Config
	logger         *logrus.Logger
}

// NewManager creates a new email manager
func NewManager(cfg *config.Config, cacheStore *cache.Store, logger *logrus.Logger) (*Manager, error) {
	accountManager, err := NewAccountManager(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create account manager: %w", err)
	}

	// Set loggers for all accounts
	for _, account := range accountManager.accounts {
		account.IMAP.SetLogger(logger)
		account.SMTP.SetLogger(logger)
	}

	return &Manager{
		accountManager: accountManager,
		store:          cacheStore,
		config:         cfg,
		logger:         logger,
	}, nil
}

// SyncAccount syncs emails from IMAP to cache for an account
func (m *Manager) SyncAccount(accountName string, folderName string) error {
	account, err := m.accountManager.GetAccount(accountName)
	if err != nil {
		return fmt.Errorf("account not found: %s", accountName)
	}
	if account == nil {
		return fmt.Errorf("account not found: %s", accountName)
	}

	// Get account ID
	accountID, err := m.store.GetAccountID(accountName)
	if err != nil {
		// Account not in cache, create it
		accountID, err = m.store.UpsertAccount(account.Config)
		if err != nil {
			return fmt.Errorf("failed to create account in cache: %w", err)
		}
	}

	// List folders if folderName is empty
	if folderName == "" {
		folders, err := account.IMAP.ListFolders()
		if err != nil {
			return fmt.Errorf("failed to list folders: %w", err)
		}

		// Sync all folders
		for _, folder := range folders {
			if err := m.syncFolder(account, accountID, folder.Name); err != nil {
				m.logger.WithError(err).WithField("folder", folder.Name).Warn("Failed to sync folder")
			}
		}
	} else {
		// Sync specific folder
		if err := m.syncFolder(account, accountID, folderName); err != nil {
			return fmt.Errorf("failed to sync folder: %w", err)
		}
	}

	return nil
}

// syncFolder syncs a single folder
func (m *Manager) syncFolder(account *Account, accountID int, folderName string) error {
	// Get folder status
	status, err := account.IMAP.GetFolderStatus(folderName)
	if err != nil {
		return fmt.Errorf("failed to get folder status: %w", err)
	}

	// Upsert folder in cache
	folderID, err := m.store.UpsertFolder(accountID, folderName, folderName, int(status.Messages))
	if err != nil {
		return fmt.Errorf("failed to upsert folder: %w", err)
	}

	// Fetch emails (recent 100 by default)
	emails, err := account.IMAP.FetchEmails(folderName, 0, 0)
	if err != nil {
		return fmt.Errorf("failed to fetch emails: %w", err)
	}

	// Store emails in cache
	for _, email := range emails {
		email.AccountID = accountID
		email.FolderID = folderID
		if err := m.store.UpsertEmail(email); err != nil {
			m.logger.WithError(err).WithField("email_id", email.UID).Warn("Failed to cache email")
		}
	}

	m.logger.WithFields(logrus.Fields{
		"account": account.Config.Name,
		"folder":  folderName,
		"count":   len(emails),
	}).Info("Synced folder")

	return nil
}

// SendEmail sends an email
func (m *Manager) SendEmail(accountName string, msg *EmailMessage) error {
	account, err := m.accountManager.GetAccount(accountName)
	if err != nil {
		return fmt.Errorf("account not found: %s", accountName)
	}
	if account == nil {
		return fmt.Errorf("account not found: %s", accountName)
	}

	if err := account.SMTP.Send(msg); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}

// Close closes all connections
func (m *Manager) Close() error {
	return m.accountManager.Close()
}

// GetAccount returns an account by name
func (m *Manager) GetAccount(name string) (*Account, error) {
	return m.accountManager.GetAccount(name)
}

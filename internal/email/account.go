package email

import (
	"github.com/Tillman32/mcp-email/internal/config"
)

// AccountManager manages multiple email accounts
type AccountManager struct {
	accounts map[string]*Account
}

// Account represents an email account with IMAP and SMTP clients
type Account struct {
	Config *config.AccountConfig
	IMAP   *IMAPClient
	SMTP   *SMTPClient
}

// NewAccountManager creates a new account manager
func NewAccountManager(cfg *config.Config) (*AccountManager, error) {
	manager := &AccountManager{
		accounts: make(map[string]*Account),
	}

	// Initialize accounts
	for i := range cfg.Accounts {
		accCfg := &cfg.Accounts[i]

		// Create IMAP client
		imapClient, err := NewIMAPClient(accCfg)
		if err != nil {
			return nil, err
		}

		// Create SMTP client
		smtpClient, err := NewSMTPClient(accCfg)
		if err != nil {
			return nil, err
		}

		account := &Account{
			Config: accCfg,
			IMAP:   imapClient,
			SMTP:   smtpClient,
		}

		manager.accounts[accCfg.Name] = account
	}

	return manager, nil
}

// GetAccount returns an account by name
func (m *AccountManager) GetAccount(name string) (*Account, error) {
	account, exists := m.accounts[name]
	if !exists {
		return nil, nil
	}
	return account, nil
}

// ListAccounts returns all account names
func (m *AccountManager) ListAccounts() []string {
	names := make([]string, 0, len(m.accounts))
	for name := range m.accounts {
		names = append(names, name)
	}
	return names
}

// Close closes all account connections
func (m *AccountManager) Close() error {
	for _, account := range m.accounts {
		if account.IMAP != nil {
			account.IMAP.Close()
		}
	}
	return nil
}

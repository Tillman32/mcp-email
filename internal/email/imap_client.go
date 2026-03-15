package email

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/jhillyerd/enmime"
	"github.com/sirupsen/logrus"

	"github.com/Tillman32/mcp-email/internal/config"
	"github.com/Tillman32/mcp-email/pkg/types"
)

// IMAPClient wraps an IMAP client connection
type IMAPClient struct {
	config    *config.AccountConfig
	client    *client.Client
	logger    *logrus.Logger
	connected bool
}

// NewIMAPClient creates a new IMAP client (does not connect immediately)
func NewIMAPClient(cfg *config.AccountConfig) (*IMAPClient, error) {
	return &IMAPClient{
		config:    cfg,
		logger:    logrus.New(),
		connected: false,
	}, nil
}

// Connect establishes a connection to the IMAP server
func (c *IMAPClient) Connect() error {
	if c.connected && c.client != nil {
		return nil
	}

	addr := fmt.Sprintf("%s:%d", c.config.IMAPHost, c.config.IMAPPort)

	// Connect to server
	cl, err := client.DialTLS(addr, &tls.Config{
		ServerName: c.config.IMAPHost,
		MinVersion: tls.VersionTLS12,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to IMAP server: %w", err)
	}

	c.client = cl

	// Login
	if err := c.client.Login(c.config.IMAPUsername, c.config.IMAPPassword); err != nil {
		c.logger.WithError(err).Error("Failed to login to IMAP server")
		c.client.Logout() //nolint:errcheck
		c.client = nil
		return fmt.Errorf("failed to login to IMAP server: %w", err)
	}

	c.connected = true
	c.logger.WithField("account", c.config.Name).Info("Connected to IMAP server")
	return nil
}

// Close closes the IMAP connection
func (c *IMAPClient) Close() error {
	if c.client != nil {
		if err := c.client.Logout(); err != nil {
			return err
		}
		c.client = nil
		c.connected = false
	}
	return nil
}

// ListFolders lists all mailboxes/folders
func (c *IMAPClient) ListFolders() ([]types.Folder, error) {
	if err := c.Connect(); err != nil {
		return nil, err
	}

	mailboxes := make(chan *imap.MailboxInfo, 10)
	done := make(chan error, 1)

	go func() {
		done <- c.client.List("", "*", mailboxes)
	}()

	var folders []types.Folder
	for m := range mailboxes {
		folder := types.Folder{
			Name: m.Name,
			Path: m.Name,
		}
		folders = append(folders, folder)
	}

	if err := <-done; err != nil {
		return nil, fmt.Errorf("failed to list folders: %w", err)
	}

	return folders, nil
}

// GetFolderStatus gets the status of a folder (message count, etc.)
func (c *IMAPClient) GetFolderStatus(folderName string) (*imap.MailboxStatus, error) {
	if err := c.Connect(); err != nil {
		return nil, err
	}

	mbox, err := c.client.Select(folderName, false)
	if err != nil {
		return nil, fmt.Errorf("failed to select folder: %w", err)
	}

	return mbox, nil
}

// FetchEmails fetches emails from a folder
func (c *IMAPClient) FetchEmails(folderName string, from, to uint32) ([]*types.Email, error) {
	if err := c.Connect(); err != nil {
		return nil, err
	}

	// Select folder
	mbox, err := c.client.Select(folderName, false)
	if err != nil {
		return nil, fmt.Errorf("failed to select folder: %w", err)
	}

	if mbox.Messages == 0 {
		return []*types.Email{}, nil
	}

	// Determine sequence range
	seqSet := new(imap.SeqSet)
	if from == 0 && to == 0 {
		// Fetch recent emails (last 100 by default)
		start := uint32(1)
		if mbox.Messages > 100 {
			start = mbox.Messages - 99
		}
		seqSet.AddRange(start, mbox.Messages)
	} else {
		seqSet.AddRange(from, to)
	}

	// Fetch messages (using RFC822 for full message content)
	items := []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchInternalDate, imap.FetchUid, imap.FetchRFC822}

	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)

	go func() {
		done <- c.client.Fetch(seqSet, items, messages)
	}()

	var emails []*types.Email
	for msg := range messages {
		email := c.parseMessage(msg, folderName)
		emails = append(emails, email)
	}

	if err := <-done; err != nil {
		return nil, fmt.Errorf("failed to fetch messages: %w", err)
	}

	return emails, nil
}

// parseMessage parses an IMAP message into our Email type
func (c *IMAPClient) parseMessage(msg *imap.Message, folderName string) *types.Email {
	email := &types.Email{
		UID:        msg.Uid,
		MessageID:  msg.Envelope.MessageId,
		Subject:    msg.Envelope.Subject,
		Date:       msg.Envelope.Date,
		FolderPath: folderName,
		Recipients: []string{},
		Headers:    make(map[string]string),
		Flags:      []string{},
	}

	// Parse sender
	if len(msg.Envelope.From) > 0 {
		addr := msg.Envelope.From[0]
		email.SenderName = addr.PersonalName
		email.SenderEmail = addr.Address()
	}

	// Parse recipients
	for _, to := range msg.Envelope.To {
		email.Recipients = append(email.Recipients, to.Address())
	}
	for _, cc := range msg.Envelope.Cc {
		email.Recipients = append(email.Recipients, cc.Address())
	}
	for _, bcc := range msg.Envelope.Bcc {
		email.Recipients = append(email.Recipients, bcc.Address())
	}

	// Parse flags
	email.Flags = append(email.Flags, msg.Flags...)

	// Parse body using RFC822 content with enmime
	if msg.Body != nil {
		c.logger.WithField("body_keys", getBodyKeys(msg.Body)).WithField("body_type", fmt.Sprintf("%T", msg.Body)).Debug("Body info")

		// Try multiple ways to access the body content
		var bodyBytes []byte

		// Method 1: Try nil key (RFC822)
		if literal, ok := msg.Body[nil]; ok {
			c.logger.Debug("Found nil key, reading RFC822 content")
			bodyBytes = c.readLiteralToBytes(literal)
		} else {
			// Method 2: Try empty BodySectionName
			emptySection := &imap.BodySectionName{}
			if literal, ok := msg.Body[emptySection]; ok {
				c.logger.Debug("Found empty section, reading content")
				bodyBytes = c.readLiteralToBytes(literal)
			} else {
				// Method 3: Try any available section
				for section, literal := range msg.Body {
					c.logger.WithField("trying_section", fmt.Sprintf("%v", section)).Debug("Trying available section")
					bodyBytes = c.readLiteralToBytes(literal)
					if len(bodyBytes) > 0 {
						break
					}
				}
			}
		}

		if len(bodyBytes) > 0 {
			c.logger.WithField("body_size", len(bodyBytes)).Debug("Body bytes read")
			if len(bodyBytes) > 0 {
				c.logger.WithField("body_preview", string(bodyBytes[:min(200, len(bodyBytes))])).Debug("Body preview")
			}

			// Try to parse with enmime
			env, err := enmime.ReadEnvelope(bytes.NewReader(bodyBytes))
			if err == nil {
				email.BodyText = env.Text
				email.BodyHTML = env.HTML
				c.logger.WithFields(logrus.Fields{
					"text_len": len(env.Text),
					"html_len": len(env.HTML),
				}).Debug("Successfully parsed with enmime")
			} else {
				// Fallback: try to extract text directly
				bodyStr := string(bodyBytes)
				email.BodyText = bodyStr
				c.logger.WithError(err).Debug("Failed to parse with enmime, using raw body")
			}
		} else {
			c.logger.Error("No body content found")
		}
	} else {
		c.logger.Error("Message body is nil")
	}

	return email
}

// SearchEmails searches for emails in a folder
func (c *IMAPClient) SearchEmails(folderName string, criteria *imap.SearchCriteria) ([]uint32, error) {
	if err := c.Connect(); err != nil {
		return nil, err
	}

	// Select folder
	_, err := c.client.Select(folderName, false)
	if err != nil {
		return nil, fmt.Errorf("failed to select folder: %w", err)
	}

	// Search
	uids, err := c.client.Search(criteria)
	if err != nil {
		return nil, fmt.Errorf("failed to search emails: %w", err)
	}

	return uids, nil
}

// SetLogger sets the logger for the client
func (c *IMAPClient) SetLogger(logger *logrus.Logger) {
	c.logger = logger
}

// getBodyKeys returns a string representation of available body section keys
func getBodyKeys(body map[*imap.BodySectionName]imap.Literal) []string {
	var keys []string
	for k := range body {
		if k == nil {
			keys = append(keys, "nil")
		} else {
			keys = append(keys, fmt.Sprintf("%v", k))
		}
	}
	return keys
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// readLiteralToBytes reads content from an IMAP literal and returns bytes
func (c *IMAPClient) readLiteralToBytes(literal imap.Literal) []byte {
	bodyBytes := make([]byte, 0, 8192)
	buf := make([]byte, 1024)
	for {
		n, err := literal.Read(buf)
		if n > 0 {
			bodyBytes = append(bodyBytes, buf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			c.logger.WithError(err).Error("Error reading literal")
			break
		}
	}
	return bodyBytes
}

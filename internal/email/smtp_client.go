package email

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/Tillman32/mcp-email/internal/config"
)

// SMTPClient wraps an SMTP client
type SMTPClient struct {
	config *config.AccountConfig
	logger *logrus.Logger
}

// EmailMessage represents an email to be sent
type EmailMessage struct {
	To          []string
	Cc          []string
	Bcc         []string
	Subject     string
	BodyText    string
	BodyHTML    string
	Attachments []Attachment
	ReplyTo     string
	InReplyTo   string
}

// Attachment represents an email attachment
type Attachment struct {
	Filename string
	Content  []byte
	MimeType string
}

// NewSMTPClient creates a new SMTP client
func NewSMTPClient(cfg *config.AccountConfig) (*SMTPClient, error) {
	return &SMTPClient{
		config: cfg,
		logger: logrus.New(),
	}, nil
}

// Send sends an email
func (c *SMTPClient) Send(msg *EmailMessage) error {
	// Create message
	emailBytes := c.createMessage(msg)

	// Connect to server
	addr := fmt.Sprintf("%s:%d", c.config.SMTPHost, c.config.SMTPPort)

	// Determine if TLS is needed
	useTLS := c.config.SMTPPort == 465

	var auth smtp.Auth
	if c.config.SMTPPassword != "" {
		auth = smtp.PlainAuth("", c.config.SMTPUsername, c.config.SMTPPassword, c.config.SMTPHost)
	}

	if useTLS {
		// TLS connection (port 465)
		conn, err := tls.Dial("tcp", addr, &tls.Config{
			ServerName: c.config.SMTPHost,
			MinVersion: tls.VersionTLS12,
		})
		if err != nil {
			return fmt.Errorf("failed to connect to SMTP server: %w", err)
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, c.config.SMTPHost)
		if err != nil {
			return fmt.Errorf("failed to create SMTP client: %w", err)
		}
		defer client.Close()

		// Auth
		if auth != nil {
			if authErr := client.Auth(auth); authErr != nil {
				return fmt.Errorf("failed to authenticate: %w", authErr)
			}
		}

		// Set sender
		if mailErr := client.Mail(c.config.SMTPUsername); mailErr != nil {
			return fmt.Errorf("failed to set sender: %w", mailErr)
		}

		// Set recipients
		recipients := append(append(msg.To, msg.Cc...), msg.Bcc...)
		for _, to := range recipients {
			if rcptErr := client.Rcpt(to); rcptErr != nil {
				return fmt.Errorf("failed to set recipient %s: %w", to, rcptErr)
			}
		}

		// Send data
		w, dataErr := client.Data()
		if dataErr != nil {
			return fmt.Errorf("failed to send data command: %w", dataErr)
		}

		if _, writeErr := w.Write(emailBytes); writeErr != nil {
			return fmt.Errorf("failed to write message: %w", writeErr)
		}

		if closeErr := w.Close(); closeErr != nil {
			return fmt.Errorf("failed to close data writer: %w", closeErr)
		}

		return client.Quit()
	} else {
		// StartTLS connection (port 587)
		client, err := smtp.Dial(addr)
		if err != nil {
			return fmt.Errorf("failed to connect to SMTP server: %w", err)
		}
		defer client.Close()

		// Start TLS
		if err := client.StartTLS(&tls.Config{
			ServerName: c.config.SMTPHost,
			MinVersion: tls.VersionTLS12,
		}); err != nil {
			return fmt.Errorf("failed to start TLS: %w", err)
		}

		// Auth
		if auth != nil {
			if authErr := client.Auth(auth); authErr != nil {
				return fmt.Errorf("failed to authenticate: %w", authErr)
			}
		}

		// Set sender
		if mailErr := client.Mail(c.config.SMTPUsername); mailErr != nil {
			return fmt.Errorf("failed to set sender: %w", mailErr)
		}

		// Set recipients
		recipients := append(append(msg.To, msg.Cc...), msg.Bcc...)
		for _, to := range recipients {
			if rcptErr := client.Rcpt(to); rcptErr != nil {
				return fmt.Errorf("failed to set recipient %s: %w", to, rcptErr)
			}
		}

		// Send data
		w, dataErr := client.Data()
		if dataErr != nil {
			return fmt.Errorf("failed to send data command: %w", dataErr)
		}

		if _, writeErr := w.Write(emailBytes); writeErr != nil {
			return fmt.Errorf("failed to write message: %w", writeErr)
		}

		if closeErr := w.Close(); closeErr != nil {
			return fmt.Errorf("failed to close data writer: %w", closeErr)
		}

		return client.Quit()
	}
}

// createMessage creates an email message in MIME format
func (c *SMTPClient) createMessage(msg *EmailMessage) []byte {
	var buf bytes.Buffer

	// Write headers manually (simpler approach)
	buf.WriteString(fmt.Sprintf("From: %s\r\n", c.config.SMTPUsername))
	buf.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(msg.To, ", ")))
	if len(msg.Cc) > 0 {
		buf.WriteString(fmt.Sprintf("Cc: %s\r\n", strings.Join(msg.Cc, ", ")))
	}
	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", msg.Subject))
	if msg.ReplyTo != "" {
		buf.WriteString(fmt.Sprintf("Reply-To: %s\r\n", msg.ReplyTo))
	}
	if msg.InReplyTo != "" {
		buf.WriteString(fmt.Sprintf("In-Reply-To: %s\r\n", msg.InReplyTo))
	}

	// Set content type
	if msg.BodyHTML != "" {
		buf.WriteString("Content-Type: text/html; charset=utf-8\r\n")
		buf.WriteString("\r\n")
		buf.WriteString(msg.BodyHTML)
	} else {
		buf.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
		buf.WriteString("\r\n")
		buf.WriteString(msg.BodyText)
	}

	return buf.Bytes()
}

// SetLogger sets the logger for the client
func (c *SMTPClient) SetLogger(logger *logrus.Logger) {
	c.logger = logger
}

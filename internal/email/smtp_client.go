package email

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"net/smtp"
	"net/textproto"
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
	recipients := append(append(msg.To, msg.Cc...), msg.Bcc...)
	return c.deliver(recipients, c.createMessage(msg))
}

// SendRaw sends pre-built RFC 2822 message bytes (e.g. a draft fetched back
// from the server, attachments included) to the given recipients.
func (c *SMTPClient) SendRaw(recipients []string, raw []byte) error {
	return c.deliver(recipients, raw)
}

// deliver transmits raw message bytes over SMTP, using implicit TLS on port
// 465 and STARTTLS otherwise.
func (c *SMTPClient) deliver(recipients []string, raw []byte) error {
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

		// Set recipients (resolved by the caller)
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

		if _, writeErr := w.Write(raw); writeErr != nil {
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

		// Set recipients (resolved by the caller)
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

		if _, writeErr := w.Write(raw); writeErr != nil {
			return fmt.Errorf("failed to write message: %w", writeErr)
		}

		if closeErr := w.Close(); closeErr != nil {
			return fmt.Errorf("failed to close data writer: %w", closeErr)
		}

		return client.Quit()
	}
}

// createMessage creates an email message in MIME format. Messages without
// attachments keep the existing simple single-part layout; messages with
// attachments use multipart/mixed with base64-encoded file parts.
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

	bodyContentType := "text/plain; charset=utf-8"
	body := msg.BodyText
	if msg.BodyHTML != "" {
		bodyContentType = "text/html; charset=utf-8"
		body = msg.BodyHTML
	}

	if len(msg.Attachments) == 0 {
		buf.WriteString(fmt.Sprintf("Content-Type: %s\r\n", bodyContentType))
		buf.WriteString("\r\n")
		buf.WriteString(body)
		return buf.Bytes()
	}

	buf.WriteString("MIME-Version: 1.0\r\n")
	mw := multipart.NewWriter(&buf)
	buf.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=%s\r\n", mw.Boundary()))
	buf.WriteString("\r\n")

	// Body part first
	bodyHeader := textproto.MIMEHeader{}
	bodyHeader.Set("Content-Type", bodyContentType)
	bodyHeader.Set("Content-Transfer-Encoding", "8bit")
	bodyPart, err := mw.CreatePart(bodyHeader)
	if err == nil {
		if _, err := io.WriteString(bodyPart, body); err != nil {
			c.logger.WithError(err).Warn("Failed to write email body part")
		}
	}

	// One base64 part per attachment
	for _, att := range msg.Attachments {
		name := sanitizeFilename(att.Filename)
		mimeType := att.MimeType
		if mimeType == "" {
			mimeType = defaultMimeType
		}
		h := textproto.MIMEHeader{}
		h.Set("Content-Type", fmt.Sprintf("%s; name=\"%s\"", mimeType, name))
		h.Set("Content-Transfer-Encoding", "base64")
		h.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", name))
		part, err := mw.CreatePart(h)
		if err != nil {
			continue
		}
		if err := writeBase64Lines(part, att.Content); err != nil {
			continue
		}
	}

	_ = mw.Close()
	return buf.Bytes()
}

// sanitizeFilename strips path separators and characters that would break a
// MIME header parameter value.
func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, `"`, "'")
	name = strings.ReplaceAll(name, "\r", "")
	name = strings.ReplaceAll(name, "\n", "")
	name = strings.TrimSpace(name)
	if name == "" || name == "." {
		return defaultAttachmentName
	}
	return name
}

// writeBase64Lines writes base64 with 76-char CRLF line breaks per RFC 2045.
func writeBase64Lines(w io.Writer, content []byte) error {
	const lineLen = 76
	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(content)))
	base64.StdEncoding.Encode(encoded, content)
	for len(encoded) > 0 {
		n := min(len(encoded), lineLen)
		if _, err := w.Write(encoded[:n]); err != nil {
			return err
		}
		if _, err := w.Write([]byte("\r\n")); err != nil {
			return err
		}
		encoded = encoded[n:]
	}
	return nil
}

// BuildMessage returns the raw RFC 2822 message bytes for a message without
// sending it. This is used to build draft messages that are stored via IMAP
// APPEND before the user is ready to send.
func (c *SMTPClient) BuildMessage(msg *EmailMessage) []byte {
	return c.createMessage(msg)
}

// SetLogger sets the logger for the client
func (c *SMTPClient) SetLogger(logger *logrus.Logger) {
	c.logger = logger
}

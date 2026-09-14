package email

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Attachment size limits. Providers commonly reject messages over 25MB;
// keep individual files modest so a draft with a few files still sends.
const (
	maxAttachmentCount = 10
	maxAttachmentBytes = 10 << 20 // 10MB per file
	maxAttachmentsSize = 25 << 20 // 25MB total
)

// Fallbacks used when a filename or MIME type can't be determined.
const (
	defaultAttachmentName = "attachment"
	defaultMimeType       = "application/octet-stream"
)

// attachmentFetchTimeout bounds how long we wait for a URL attachment.
const attachmentFetchTimeout = 15 * time.Second

// ParseAttachmentSources normalizes the MCP "attachments" parameter (a JSON
// array of strings, which may decode as []interface{}) into a plain slice.
// Each entry is either a local file path or an http(s) URL.
func ParseAttachmentSources(v interface{}) ([]string, error) {
	if v == nil {
		return nil, nil
	}
	switch src := v.(type) {
	case []string:
		return src, nil
	case []interface{}:
		out := make([]string, 0, len(src))
		for i, item := range src {
			s, ok := item.(string)
			if !ok || strings.TrimSpace(s) == "" {
				return nil, fmt.Errorf("attachments[%d] must be a non-empty string (file path or URL)", i)
			}
			out = append(out, s)
		}
		return out, nil
	case string:
		if strings.TrimSpace(src) == "" {
			return nil, nil
		}
		return []string{src}, nil
	default:
		return nil, fmt.Errorf("attachments must be an array of file paths/URLs")
	}
}

// LoadAttachments reads each source (local file path or http(s) URL) into an
// Attachment with a detected filename and MIME type. Limits are enforced per
// file and in total so a bad input can't blow up memory or the message.
func LoadAttachments(sources []string) ([]Attachment, error) {
	if len(sources) > maxAttachmentCount {
		return nil, fmt.Errorf("too many attachments: max %d", maxAttachmentCount)
	}

	attachments := make([]Attachment, 0, len(sources))
	var total int64
	for _, src := range sources {
		var att Attachment
		var err error
		if isURL(src) {
			att, err = loadURLAttachment(src)
		} else {
			att, err = loadFileAttachment(src)
		}
		if err != nil {
			return nil, err
		}
		total += int64(len(att.Content))
		if total > maxAttachmentsSize {
			return nil, fmt.Errorf("attachments exceed %dMB total", maxAttachmentsSize>>20)
		}
		attachments = append(attachments, att)
	}
	return attachments, nil
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func loadFileAttachment(path string) (Attachment, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Attachment{}, fmt.Errorf("cannot read attachment %q: %w", path, err)
	}
	if info.IsDir() {
		return Attachment{}, fmt.Errorf("attachment %q is a directory, not a file", path)
	}

	f, err := os.Open(path)
	if err != nil {
		return Attachment{}, fmt.Errorf("cannot open attachment %q: %w", path, err)
	}
	defer f.Close() //nolint:errcheck

	content, err := io.ReadAll(io.LimitReader(f, maxAttachmentBytes+1))
	if err != nil {
		return Attachment{}, fmt.Errorf("cannot read attachment %q: %w", path, err)
	}
	if len(content) > maxAttachmentBytes {
		return Attachment{}, fmt.Errorf("attachment %q exceeds %dMB", path, maxAttachmentBytes>>20)
	}

	name := filepath.Base(path)
	return Attachment{
		Filename: name,
		Content:  content,
		MimeType: detectMIMEType(name, content),
	}, nil
}

func loadURLAttachment(rawURL string) (Attachment, error) {
	client := &http.Client{Timeout: attachmentFetchTimeout}
	resp, err := client.Get(rawURL) //nolint:gosec,noctx
	if err != nil {
		return Attachment{}, fmt.Errorf("cannot fetch attachment %q: %w", rawURL, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Attachment{}, fmt.Errorf("cannot fetch attachment %q: HTTP %s", rawURL, resp.Status)
	}

	content, err := io.ReadAll(io.LimitReader(resp.Body, maxAttachmentBytes+1))
	if err != nil {
		return Attachment{}, fmt.Errorf("cannot read attachment %q: %w", rawURL, err)
	}
	if len(content) > maxAttachmentBytes {
		return Attachment{}, fmt.Errorf("attachment %q exceeds %dMB", rawURL, maxAttachmentBytes>>20)
	}

	name := filenameFromURL(rawURL)
	mimeType := strings.TrimSpace(strings.SplitN(resp.Header.Get("Content-Type"), ";", 2)[0])
	if mimeType == "" || mimeType == defaultMimeType {
		mimeType = detectMIMEType(name, content)
	}
	return Attachment{Filename: name, Content: content, MimeType: mimeType}, nil
}

func filenameFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "attachment"
	}
	name := filepath.Base(strings.TrimSuffix(u.Path, "/"))
	if name == "" || name == "." || name == "/" {
		return defaultAttachmentName
	}
	return name
}

// detectMIMEType prefers the file extension, then sniffs content, and falls
// back to application/octet-stream.
func detectMIMEType(filename string, content []byte) string {
	if ext := strings.ToLower(filepath.Ext(filename)); ext != "" {
		if mt := mime.TypeByExtension(ext); mt != "" {
			if i := strings.Index(mt, ";"); i >= 0 {
				mt = strings.TrimSpace(mt[:i])
			}
			return mt
		}
	}
	if len(content) > 0 {
		return http.DetectContentType(content)
	}
	return defaultMimeType
}

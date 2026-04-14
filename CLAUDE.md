# MCP Email Server

An MCP (Model Context Protocol) server that lets AI assistants interact with email accounts via IMAP/SMTP. Supports multiple accounts, SQLite-backed caching with FTS5 full-text search, and unsubscribe automation. No OAuth — app passwords only.

## Commands

```bash
# Build
npm run build          # go build → ./bin/mcp-email-server
npm run build:docker   # Docker multi-stage build

# Run
npm run dev            # go run ./cmd/server (requires .env)
npm run dev:docker     # Docker run with port 8080

# Test & Lint
go test ./...
golangci-lint run
npm test               # MCP inspector (requires .env)
```

## Architecture

```
cmd/server/main.go          # Entry point: init config → cache → email manager → MCP server
internal/
  config/config.go          # Env var config (single-account or ACCOUNT_N_* multi-account)
  email/                    # IMAP client, SMTP client, account manager, auto-sync
  cache/                    # SQLite + FTS5: schema, store, search
  mcp/server.go             # MCP stdio transport, request routing
  tools/                    # One file per MCP tool (registry.go registers all)
pkg/types/                  # Shared structs: Email, EmailSummary, Account
```

## MCP Tools

| Tool | File | Purpose |
|------|------|---------|
| `list_folders` | `tools/list_folders.go` | List mailbox folders |
| `search_emails` | `tools/search.go` | FTS5 search over cached emails |
| `get_email` | `tools/get_email.go` | Fetch full email by ID |
| `send_email` | `tools/send.go` | Send via SMTP |
| `find_unsubscribe_link` | `tools/find_unsubscribe_link.go` | Extract unsubscribe links |
| `get_sender_stats` | `tools/get_sender_stats.go` | Per-sender statistics |
| `execute_unsubscribe` | `tools/execute_unsubscribe.go` | HTTP GET/POST or mailto unsubscribe |

## Adding a New Tool

1. Create `internal/tools/<tool_name>.go` — implement the handler func
2. Register it in `internal/tools/registry.go`
3. Add any new cache columns/tables to `internal/cache/schema.go`
4. Add cache methods to `internal/cache/store.go`

## Configuration

Copy `.env.example` to `.env`. Key vars:

```
EMAIL_ADDRESS=you@example.com
EMAIL_PASSWORD=app-password
IMAP_HOST=imap.example.com
IMAP_PORT=993
SMTP_HOST=smtp.example.com
SMTP_PORT=587

# Multi-account: prefix with ACCOUNT_N_
ACCOUNT_1_EMAIL_ADDRESS=...
ACCOUNT_2_EMAIL_ADDRESS=...

CACHE_PATH=~/.mcp-email/cache.db  # SQLite path
LOG_LEVEL=info
```

## Key Implementation Details

- **MCP protocol**: Custom JSON-over-stdio implementation (no external MCP SDK)
- **SQLite**: Pure Go (`modernc.org/sqlite`, no cgo). FTS5 for full-text search.
- **Email parsing**: `jhillyerd/enmime` for MIME, `PuerkitoBio/goquery` for HTML (unsubscribe link extraction)
- **Linter**: `golangci-lint` with errcheck, gosec, staticcheck, gofmt, goimports, etc. Line limit: 120. Config: `.golangci.yml`
- **Module**: `github.com/brandon/mcp-email`

## Current Feature Status (feat/unsubscribe branch)

- Phase 1 ✅ — `find_unsubscribe_link`, `get_sender_stats`
- Phase 2 ✅ — `execute_unsubscribe` (HTTP GET/POST + mailto)
- Phase 3 ⏳ — Agent definition + MCP config examples
- Phase 4 ⏳ — `run.sh`, OS keychain credential storage, README updates

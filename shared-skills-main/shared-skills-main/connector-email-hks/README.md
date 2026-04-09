# connector-email-hks

Outlook connector + OpenClaw skill using Microsoft Graph (account-agnostic).

Implements Microsoft Graph email operations in Go:
- send email (To/CC/BCC, HTML/text)
- send with attachments
- list/search inbox
- read full message
- download attachments
- reply / reply-all

---

## Repository structure

```text
connector-email-hks/
├── SKILL.md
├── README.md
└── scripts/
    ├── go.mod
    ├── main.go
    └── outlook_send_mail.py   # legacy fallback
```

---

## Requirements

- Go 1.22+
- Microsoft app registration with Graph permissions
- OAuth env vars (local only, not committed):
  - `TENANT_ID`
  - `CLIENT_ID`
  - `CLIENT_SECRET`
  - optional `REDIRECT_URI` (default `http://localhost`)

Token cache path (default):
- `~/.openclaw/outlook_token_cache.json`

Named profile cache path:
- `~/.openclaw/outlook_token_cache_<profile>.json`

---

## Identity guardrails (important)

- Use `whoami` to verify which mailbox is actually authenticated.
- Use `--expect-user <email>` on `list/read/send/reply/download-attachments` to fail closed on identity mismatch.
- Command output now includes `AUTHENTICATED_AS=...` on stderr so automation can verify identity before trusting results.

Example:

```bash
cd skills/connector-email-hks/scripts
go run . whoami --profile "risk-agent" --expect-user "risk-agent@hksglobal.group"
go run . list --profile "risk-agent" --expect-user "risk-agent@hksglobal.group" --top 10
```

## Create a user profile (first login)

A **profile** here means a local token-cache name (for example: `henry`, `risk-agent`).
It does **not** create a user in Azure; it isolates local sign-in sessions.

1) Load required parameters:

```bash
set -a; source .secrets/outlook.env; set +a
```

Required parameters in env:
- `TENANT_ID`
- `CLIENT_ID`
- `CLIENT_SECRET`
- optional `REDIRECT_URI` (default: `http://localhost`)

2) Run any command with a profile name (this triggers login on first use):

```bash
cd skills/connector-email-hks/scripts
go run . list --profile "henry" --top 5
```

3) Complete browser sign-in when prompted, then paste redirected URL.

4) Reuse the same profile for all future commands:

```bash
go run . send --profile "henry" --to "a@b.com" --subject "Hi" --body "Hello" --text
```

Optional default profile via env:

```bash
export OUTLOOK_PROFILE=henry
```

---

## Quick start

```bash
set -a; source .secrets/outlook.env; set +a
cd skills/connector-email-hks/scripts
go run . help
```

---

## Commands

### Send

```bash
go run . send \
  --to "recipient@example.com" \
  --cc "cc1@example.com" \
  --subject "Subject" \
  --body "<p>Hello</p>"
```

Send with attachments:

```bash
go run . send \
  --to "recipient@example.com" \
  --subject "Report" \
  --body "<p>Attached.</p>" \
  --attach "/tmp/report.pdf,/tmp/data.csv"
```

Plain text:

```bash
go run . send --to "recipient@example.com" --subject "Hi" --body "Hello" --text --expect-user "henry@hksglobal.group"
```

### Who am I (verify authenticated mailbox)

```bash
go run . whoami --profile "risk-agent"
go run . whoami --profile "risk-agent" --expect-user "risk-agent@hksglobal.group"
```

### List / Search

```bash
go run . list --folder Inbox --top 20 --expect-user "henry@hksglobal.group"
go run . list --query "from:y.borisova@hksglobal.group" --expect-user "henry@hksglobal.group"
go run . list --profile "risk-agent" --expect-user "risk-agent@hksglobal.group" --top 20
```

### Read

```bash
go run . read --id "<message-id>" --expect-user "henry@hksglobal.group"
go run . read --id "<message-id>" --attachments --expect-user "henry@hksglobal.group"
```

### Download attachments

```bash
go run . download-attachments --id "<message-id>" --out "./downloads"
```

### Reply

```bash
go run . reply --id "<message-id>" --body "<p>Thanks.</p>" --expect-user "henry@hksglobal.group"
go run . reply --id "<message-id>" --body "<p>Thanks all.</p>" --reply-all --expect-user "henry@hksglobal.group"
```

---

## Security notes

- Do not commit `.env` or `.secrets/` files.
- `.gitignore` excludes common secret paths and build artifacts.
- Keep OAuth client secret in local secure env only.

---

## Status

Current version supports operational send + inbox workflows for daily execution and automation tasks.

---

## Development roadmap

### Phase 1 (done)
- [x] Port connector to Go
- [x] Send email (HTML/text, To/CC/BCC)
- [x] Add file attachments to outgoing mail
- [x] List/search inbox
- [x] Read full messages
- [x] Download attachments
- [x] Reply / reply-all

### Phase 2 (next)
- [ ] Add thread-aware reply helpers (reply by conversation id)
- [ ] Add message flags/actions (mark read/unread, move folder)
- [ ] Add richer filters (`from`, date range, hasAttachments, unread)
- [ ] Add optional JSONL export for audit pipelines

### Phase 3 (optional hardening)
- [ ] Add retry/backoff for transient Graph failures
- [ ] Add structured error codes for automation workflows
- [ ] Add unit tests for auth/cache/Graph request layers

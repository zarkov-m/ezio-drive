---
name: connector-email-hks
description: Outlook operations via local Go CLI + Microsoft Graph. Supports drafting/sending emails (To/CC/BCC, HTML/text, attachments), inbox listing/search, message read, attachment download, and reply/reply-all. Use when asked to send/read/search Outlook emails from this machine.
---

# Outlook Email (Graph) for Mitko

Use this skill to check and manage Mitko's Outlook mailbox with the bundled Go CLI.

## Script

- `scripts/main.go`

## Required environment

```bash
set -a; source .secrets/outlook.env; set +a
```

Required vars:
- `TENANT_ID`
- `CLIENT_ID`
- `CLIENT_SECRET`
- Optional: `REDIRECT_URI` (default `http://localhost`)

## Core commands

### Send email

```bash
set -a; source .secrets/outlook.env; set +a
cd skills/connector-email-hks/scripts

go run . send \
  --to "recipient@example.com" \
  --cc "cc1@example.com,cc2@example.com" \
  --bcc "hidden1@example.com,hidden2@example.com" \
  --subject "Subject here" \
  --body "<p>HTML body here</p>" \
  --attach "/path/file1.pdf,/path/file2.csv" \
  --expect-user "m.zarkov@hksglobal.group"
```

For plain text, add `--text`.

### Verify authenticated mailbox (required before mailbox claims)

```bash
go run . whoami --cache ~/.openclaw/outlook_token_cache_<profile>.json
go run . whoami --cache ~/.openclaw/outlook_token_cache_<profile>.json --expect-user "user@company.com"
```

### List/search inbox

```bash
go run . list --folder "Inbox" --top 20 --expect-user "m.zarkov@hksglobal.group"
go run . list --query "isRead eq false" --expect-user "m.zarkov@hksglobal.group"
go run . list --profile "mitko" --expect-user "m.zarkov@hksglobal.group" --top 20
```

### Read a message

```bash
go run . read --id "<message-id>" --expect-user "m.zarkov@hksglobal.group"
go run . read --id "<message-id>" --attachments --expect-user "m.zarkov@hksglobal.group"
```

### Download attachments

```bash
go run . download-attachments --id "<message-id>" --out "./downloads" --expect-user "m.zarkov@hksglobal.group"
```

### Reply / reply-all

```bash
go run . reply --id "<message-id>" --body "<p>Thanks, noted.</p>" --expect-user "m.zarkov@hksglobal.group"
go run . reply --id "<message-id>" --body "<p>Thanks all.</p>" --reply-all --expect-user "m.zarkov@hksglobal.group"
```

## Auth behavior

- Token cache defaults:
  - default profile: `~/.openclaw/outlook_token_cache.json`
  - named profile (`--profile mitko` or `OUTLOOK_PROFILE=mitko`): `~/.openclaw/outlook_token_cache_mitko.json`
- `--cache` always overrides profile-based default paths
- Reuses valid cached access token when available
- Falls back to refresh-token grant when possible
- If no usable token exists, starts interactive authorization-code login flow

## Validation / CI

```bash
cd skills/connector-email-hks/scripts
go test ./...
go build ./...
```

GitHub Actions workflow: `.github/workflows/go-ci.yml`

## Safety

- Always run `whoami` before claiming mailbox access in replies.
- Always use `--expect-user m.zarkov@hksglobal.group` for Mitko's mailbox operations unless explicitly switching accounts.
- Treat `AUTHENTICATED_AS=...` output as mandatory evidence of identity, do not claim success without command output.
- Prefer unread checks and inbox summaries first, before broader mailbox actions.
- Always confirm recipients (To/CC/BCC), subject, and body before send.
- Do not send external emails without user approval.

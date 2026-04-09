---
name: connector-onedrive
description: "Manage OneDrive and SharePoint files via Microsoft Graph using a Go CLI: browse folders, search, stat/read files, upload/download, move/copy, safe-delete to review folder, report folder space, organize files by extension, and resolve raw SharePoint/OneDrive shared URLs into drive items. Use when the user asks to operate OneDrive or SharePoint files, folders, structure, or shared links from chat."
---

# connector-onedrive (Go)

Use the bundled Go CLI:
- `skills/connector-onedrive/scripts/main.go`

## Auth / env

Load env first:

```bash
set -a; source .secrets/outlook.env; set +a
```

Expected vars:
- `TENANT_ID`
- `CLIENT_ID`
- Optional: `CLIENT_SECRET` (recommended; enables auth-code flow)
- Optional: `REDIRECT_URI` (default: `http://localhost`)

Token cache:
- default `~/.openclaw/onedrive_token_cache.json`

## Default delete behavior (important)

When user says **delete**, use safe-delete (move to `Deleted-For-Review`) unless user explicitly asks permanent deletion.

## Commands

```bash
cd skills/connector-onedrive/scripts

# identity / quota
# prefer a per-user cache file in multi-user setups
go run . whoami --cache ~/.openclaw/onedrive_token_cache_<profile>.json
go run . quota --cache ~/.openclaw/onedrive_token_cache_<profile>.json

# browse / inspect / search
# local OneDrive paths work best with normal /path syntax
go run . ls /
go run . stat /Documents/report.pdf
go run . search "invoice" --top 50

# SharePoint / shared-link flow
# IMPORTANT: shared links are currently reliable for ls/stat, but file open/download may require
# a resolved item id from the listing instead of a guessed direct path.
go run . stat 'https://wsnan.sharepoint.com/shared?id=%2Fsites%2FDMC%2FShared%20Documents%2FPayPal%20Co%2DMarketing%20Articles&listurl=...'
go run . ls 'https://wsnan.sharepoint.com/shared?id=%2Fsites%2FDMC%2FShared%20Documents%2FPayPal%20Co%2DMarketing%20Articles&listurl=...'
# then prefer the returned item id for follow-up operations:
# go run . stat id:<item-id>
# go run . download id:<item-id> ./downloads/file.bin
# go run . cat id:<item-id> --max-bytes 200000

# read / transfer
# cat is only safe for text-like files. For docx/pdf/other binary formats, download first,
# then use a local extractor/model step to read contents.
go run . cat /Notes/todo.txt --max-bytes 200000
go run . download /Documents/report.pdf ./downloads/report.pdf
go run . upload ./local.pdf /Documents/local.pdf --large-threshold-mb 8

# structure changes
go run . mkdir /Archive/2026 --parents
go run . move /Documents/a.pdf /Archive/a.pdf
go run . copy /Documents/a.pdf /Archive/a.pdf --timeout 300

# safe delete (default)
go run . rm /Documents/a.pdf

# permanent delete (only if explicitly requested)
go run . rm /Documents/a.pdf --permanent

# space / organization
go run . report-space / --top-files 20 --top-ext 15 --max-items 50000
go run . organize-by-extension /Downloads --recursive
go run . organize-by-extension /Downloads --recursive --execute --skip "jpg,png,no_extension"
```

## Operational guidance / current limitations

### 1. Prefer item-id follow-ups after shared-link listings

When starting from a SharePoint shared URL:
- use `ls <shared-url>` or `stat <shared-url>` first
- capture the returned `id:<item-id>` values
- use `id:<item-id>` for `stat`, `download`, and `cat`
- do **not** construct guessed child file URLs/paths from the browser address bar unless already verified

Reason: shared-link folder access may work while direct child-file path resolution still fails.

### 2. Do not use `cat` on `.docx` / `.pdf` expecting readable text

`cat` returns raw file bytes from Graph content endpoints.
For binary formats like `.docx` and `.pdf`, that means unreadable binary data.
Preferred flow:
- `download id:<item-id> ./downloads/<filename>`
- then extract text locally with a suitable tool/model

### 3. OneDrive-root vs SharePoint-library behavior differs

Commands that work cleanly against `/me/drive` paths may still be unreliable when the target originated from:
- a SharePoint shared link
- a site/library path
- a resolved folder whose children are later addressed by guessed path instead of item id

Treat these as different contexts unless proven otherwise.

### 4. Recommended future improvement

The connector should grow a first-class command such as:
- `read id:<item-id>`
- or `download-from-listing <shared-url> <filename>`

so callers can move from listing → exact item reference → file content without manual path reconstruction.

## Validation / CI

```bash
cd skills/connector-onedrive/scripts
go test ./...
go build ./...
```

GitHub Actions workflow: `.github/workflows/go-ci.yml`
/go-ci.yml`

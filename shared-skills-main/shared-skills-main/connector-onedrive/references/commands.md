# connector-onedrive — command reference (Go)

CLI entrypoint:

- `go run skills/connector-onedrive/scripts/main.go <command> ...`

## Auth / env

Default env file used by CLI:

- `.secrets/outlook.env`

Required:
- `TENANT_ID`
- `CLIENT_ID`

Optional:
- `CLIENT_SECRET`
- `REDIRECT_URI` (default `http://localhost`)

Token cache default:
- `~/.openclaw/onedrive_token_cache.json`

Preferred in multi-user setups:
- `--cache ~/.openclaw/onedrive_token_cache_<user>.json`
- keep one cache file per user and reuse that path consistently

## Addressing

- path: `/Folder/Sub/file.txt`
- id: `id:<driveItemId>`
- SharePoint/OneDrive sharing URL: `https://...sharepoint.com/...`

Supported URL targets include ordinary sharing links and `.../shared?id=%2Fsites%2F...` folder/file links. URL targets are supported by `ls`, `stat`, `cat`, and `download`.

## Commands

### whoami / quota

- `whoami`
- `quota`

### ls / stat / search

- `ls /`
- `stat /Documents/report.pdf`
- `search "invoice" --top 50`

### cat / download / upload

- `cat /Notes/todo.txt --max-bytes 200000`
- `download /Documents/report.pdf ./downloads/report.pdf`
- `upload ./local.pdf /Documents/local.pdf --large-threshold-mb 8`

### mkdir / move / copy

- `mkdir /Archive/2026 --parents`
- `move /Documents/a.pdf /Archive/a.pdf`
- `copy /Documents/a.pdf /Archive/a.pdf --timeout 300`

### rm (safe default)

- safe delete: `rm /Documents/a.pdf`
- permanent: `rm /Documents/a.pdf --permanent`
- custom review folder: `rm /Documents/a.pdf --review-folder Deleted-For-Review`

### report-space

- `report-space / --top-files 20 --top-ext 15 --max-items 50000`

### organize-by-extension

- dry-run: `organize-by-extension /Downloads --recursive`
- execute: `organize-by-extension /Downloads --recursive --execute`
- skip set: `organize-by-extension /Downloads --skip "jpg,png,no_extension"`

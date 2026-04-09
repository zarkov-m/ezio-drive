# connector-onedrive

Go-first OneDrive connector/skill for OpenClaw using Microsoft Graph.

## Features

- whoami / quota
- list, stat, search
- read (`cat`), download, upload (small + chunked large upload)
- mkdir (with `--parents`), move, copy
- safe-delete (`rm` moves to `Deleted-For-Review`)
- permanent delete (`rm --permanent`)
- folder space analysis (`report-space`)
- organize files by extension (`organize-by-extension` dry-run/execute)
- resolve SharePoint/OneDrive shared URLs directly in `ls`, `stat`, `cat`, and `download`

## Quick start

```bash
git clone https://github.com/RA-AI-Internal-Tools/connector-onedrive.git
cd connector-onedrive
cp .env.example .env
set -a; source .env; set +a

cd scripts
go run . whoami
```

On first run, login consent opens in browser and asks you to paste redirected URL.

## Required parameters

Load from env (recommended):

- `TENANT_ID`
- `CLIENT_ID`

Optional:
- `CLIENT_SECRET`
- `REDIRECT_URI` (default: `http://localhost`)

## Create a user profile (first login)

This connector does not have a `--profile` flag.
Use a dedicated `--cache` file per user profile.

Example profile: `henry`

```bash
set -a; source .env; set +a
cd scripts

go run . whoami --cache ~/.openclaw/onedrive_token_cache_henry.json
```

On first run, complete login/consent. The token is then stored in that cache file.
Reuse the same cache path for all commands of that profile.

Example profile: `risk-agent`

```bash
go run . whoami --cache ~/.openclaw/onedrive_token_cache_risk_agent.json
```

## Security

- No credentials are stored in this repository.
- Keep `.env` local (ignored by git).
- Token cache default: `~/.openclaw/onedrive_token_cache.json`.

## Validation / CI

```bash
make test
make build
```

or directly:

```bash
cd scripts
go test ./...
go build ./...
```

GitHub Actions workflow: `.github/workflows/go-ci.yml`

## Packaging

```bash
# local binary
make build

# docker image
make docker-build

# cross-platform release artifacts (requires goreleaser)
make release-snapshot
```

Packaging files included:
- `Makefile`
- `Dockerfile`
- `.goreleaser.yml`

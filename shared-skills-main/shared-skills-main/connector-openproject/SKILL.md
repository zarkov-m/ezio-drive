---
name: openproject-ops
description: "OpenProject API v3 operations skill (Go CLI) for projects and work packages: list/view/create/update/delete tasks, project snapshots, users/statuses, member activity, weekly reports, endpoint discovery, generic API calls, and token-based permission inspection. Use when asked to operate OpenProject, onboard a new user token, or verify what a token can/cannot do."
---

# OpenProject Ops (Go)

Use the bundled Go CLI in `scripts/openproject_ops.go`.

## Setup (URL + token)

Preferred config file:

`.secrets/openproject.config.json`

```json
{
  "base_url": "https://your-openproject.example.com/",
  "token": "PASTE_OPENPROJECT_API_TOKEN_HERE",
  "api_path": "/api/v3",
  "parallel": 4,
  "page_size": 200
}
```

For HKS default deployment, canonical base URL is:
- `https://pm.radioactive.ac/`

Generate template:

```bash
cd scripts
go run . init-config
```

Environment overrides:
- `OPENPROJECT_BASE_URL`
- `OPENPROJECT_TOKEN`
- `OPENPROJECT_API_PATH`
- `OPENPROJECT_PARALLEL`
- `OPENPROJECT_PAGE_SIZE`
- `OPENPROJECT_CONFIG_FILE`
- `OPENPROJECT_ENV_FILE`

## First run for any new token

```bash
cd scripts
go run . permissions
go run . permissions --project frontend
```

## API compatibility

OpenAPI discovery fallback order:
- `/api/v3/openapi.json`
- `/api/v3/spec.json`
- `/docs/api/v3/spec.json`

## Core commands

```bash
cd scripts

go run . projects
go run . project-status --project frontend

go run . tasks-list --project frontend --limit 20
go run . task-view --id 9117
go run . task-create --project frontend --title "Fix login" --description "..."
go run . task-update --id 9117 --fields-json '{"percentageDone":50}'
go run . task-delete --id 9999

go run . users
go run . statuses

go run . member-activity --project frontend --from 2026-02-01 --to 2026-02-20
go run . weekly-report --project frontend --week-start 2026-02-16 --out report.md

go run . endpoints --search work_packages
go run . api-call --method GET --path /projects --query-json '{"pageSize":5}'
```

## Performance tuning

- `OPENPROJECT_PARALLEL` (default `4`, max `16`)
- `OPENPROJECT_PAGE_SIZE` (default `200`, max `1000`)

## Validation / CI

```bash
cd scripts
go test ./...
go build ./...
```

GitHub Actions workflow: `.github/workflows/go-ci.yml`

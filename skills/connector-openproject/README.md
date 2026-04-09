# OpenProject Ops (Go)

A fast, portable OpenProject API v3 CLI and OpenClaw skill for:
- project operations
- work package (task) automation
- reporting
- endpoint discovery
- token permission inspection

Designed so each user can install the same skill and configure **their own URL/token**.

---

## Features

- **Projects**: list and status summary
- **Tasks / Work Packages**: list, view, create, update, delete
- **People & workflow**: list users and statuses
- **Reporting**: member activity + weekly markdown report
- **API exploration**:
  - `endpoints` (from OpenAPI spec)
  - `api-call` (generic caller)
- **Permissions intelligence**:
  - `permissions` command inspects token-based access
  - helps users understand what they can/can’t do
- **Performance**:
  - parallel pagination
  - connection pooling
  - tunable page size and worker count

---

## Project Structure

```text
skills/openproject-ops/
├── SKILL.md
├── README.md
├── openproject.config.example.json
└── scripts/
    ├── go.mod
    └── openproject_ops.go
```

---

## Requirements

- Go 1.22+
- OpenProject API token

---

## Configuration (user-editable)

Preferred config file:

`/.secrets/openproject.config.json`

```json
{
  "base_url": "https://your-openproject.example.com/",
  "token": "PASTE_OPENPROJECT_API_TOKEN_HERE",
  "api_path": "/api/v3",
  "parallel": 4,
  "page_size": 200
}
```

Generate template automatically:

```bash
cd skills/openproject-ops/scripts
go run . init-config
```

### Env vars (optional / override)

- `OPENPROJECT_BASE_URL`
- `OPENPROJECT_TOKEN`
- `OPENPROJECT_API_PATH`
- `OPENPROJECT_CONFIG_FILE`
- `OPENPROJECT_ENV_FILE`
- `OPENPROJECT_PARALLEL`
- `OPENPROJECT_PAGE_SIZE`

Load order:
1. `.secrets/openproject.config.json`
2. `.secrets/openproject.env`
3. shell env vars (**highest priority**)

### Canonical URL (HKS deployment)

For current HKS operations, use:
- `https://pm.radioactive.ac/`

Example `.secrets/openproject.config.json`:

```json
{
  "base_url": "https://pm.radioactive.ac/",
  "token": "PASTE_OPENPROJECT_API_TOKEN_HERE",
  "api_path": "/api/v3"
}
```

---

## First-time Onboarding (new user/token)

1. Install/copy this skill folder
2. Add `.secrets/openproject.config.json` with `base_url` + `token`
3. Run:

```bash
cd skills/openproject-ops/scripts
go run . permissions
```

Optional project-scoped permission check:

```bash
go run . permissions --project frontend
```

This outputs what the current token is likely allowed to do in this tool.

---

## Commands

### Core

```bash
go run . version
go run . projects
go run . project-status --project frontend
```

### Tasks / Work Packages

```bash
go run . tasks-list --project frontend --limit 20
go run . task-view --id 9117
go run . task-create --project frontend --title "Fix login" --description "..."
go run . task-update --id 9117 --fields-json '{"percentageDone":50}'
go run . task-delete --id 9999
```

### Users / Statuses

```bash
go run . users
go run . statuses
```

### Reporting

```bash
go run . member-activity --project frontend --from 2026-02-01 --to 2026-02-20
go run . weekly-report --project frontend --week-start 2026-02-16 --out report.md
```

### API Discovery + Generic Access

```bash
go run . endpoints --search work_packages
go run . api-call --method GET --path /projects --query-json '{"pageSize":5}'
```

OpenAPI spec discovery now supports multiple endpoints for compatibility with current docs and instance variants:
- `/api/v3/openapi.json`
- `/api/v3/spec.json`
- `/docs/api/v3/spec.json`

### Permissions + Config bootstrap

```bash
go run . permissions
go run . permissions --project frontend
go run . init-config
```

---

## JSON Output

Most list/read commands support `--json`:

```bash
go run . users --json
go run . tasks-list --project frontend --json
go run . permissions --json
```

---

## Performance Tuning

- `OPENPROJECT_PARALLEL` (default `4`, max `16`)
- `OPENPROJECT_PAGE_SIZE` (default `200`, max `1000`)

Suggested starting values:
- normal instance: `parallel=4`, `page_size=200`
- constrained machine: `parallel=1..2`
- very large collections: test `parallel=6..8`

---

## Security Notes

- Keep real tokens only in `.secrets/` or env vars
- Do **not** commit user token files
- `openproject.config.example.json` is safe to commit (placeholder token)

---

## Portability (Agent to Agent)

This skill is intentionally portable:
- same code/skill can be used by different agents
- each user provides own `base_url` + `token`
- `permissions` command validates effective access immediately

---

## License

Private/internal use unless you add an explicit OSS license.

---
name: connector-figma
description: Figma REST connector via local Go CLI. Supports reading files/nodes/images/comments and writing comments for review collaboration.
---

# Figma Connector (Go)

Use this skill to query Figma files and collaborate through comments.

## Script

- `scripts/main.go`

## Required environment

```bash
set -a; source .secrets/figma.env; set +a
```

Required vars:
- `FIGMA_TOKEN` (Personal access token)

Optional:
- `FIGMA_API_BASE` (default: `https://api.figma.com/v1`)

## Core commands

```bash
cd skills/connector-figma/scripts

# Read file metadata + pages
go run . file --key "<FILE_KEY>"

# Read specific nodes
go run . nodes --key "<FILE_KEY>" --ids "12:34,56:78"

# Export node images
go run . images --key "<FILE_KEY>" --ids "12:34,56:78" --format png --scale 2

# List comments
go run . comments-list --key "<FILE_KEY>"

# Add comment (write)
go run . comments-add --key "<FILE_KEY>" --message "Please refine spacing here" --x 120 --y 340
```

## Notes

- REST API is excellent for reading design structure and adding review comments.
- Directly creating/editing frames/components is not covered by this REST connector.
- For full design mutations, use Figma plugin/runtime workflows (can be added as next phase).

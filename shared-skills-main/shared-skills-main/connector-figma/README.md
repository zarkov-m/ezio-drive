# connector-figma

Figma REST connector in Go for reading design files and collaborating via comments.

## Features
- Read file metadata and page structure
- Read selected nodes
- Export image URLs for nodes
- List comments
- Add comments with coordinates

## Setup

Create env:

```bash
export FIGMA_TOKEN="your_figma_token"
# optional
# export FIGMA_API_BASE="https://api.figma.com/v1"
```

## Usage

```bash
cd scripts

go run . file --key <FILE_KEY>
go run . nodes --key <FILE_KEY> --ids "1:2,3:4"
go run . images --key <FILE_KEY> --ids "1:2" --format png --scale 2
go run . comments-list --key <FILE_KEY>
go run . comments-add --key <FILE_KEY> --message "Please update spacing" --x 120 --y 300
```

## Note
Figma `design` files are supported via REST file endpoints. `make` files are not supported by those endpoints.

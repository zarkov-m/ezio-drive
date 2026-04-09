# syntax=docker/dockerfile:1

FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY scripts/go.mod scripts/go.sum ./scripts/
RUN cd scripts && go mod download
COPY scripts ./scripts
RUN cd scripts && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/connector-onedrive .

FROM gcr.io/distroless/static:nonroot
WORKDIR /app
COPY --from=builder /out/connector-onedrive /usr/local/bin/connector-onedrive
ENTRYPOINT ["/usr/local/bin/connector-onedrive"]

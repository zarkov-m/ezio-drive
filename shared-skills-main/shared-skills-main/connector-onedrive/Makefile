APP := connector-onedrive
PKG := ./scripts
BIN_DIR := ./bin

.PHONY: fmt test build run clean docker-build release-snapshot

fmt:
	cd scripts && gofmt -w .

test:
	cd scripts && go test ./...

build:
	mkdir -p $(BIN_DIR)
	cd scripts && go build -o ../bin/$(APP) .

run:
	cd scripts && go run . whoami

clean:
	rm -rf $(BIN_DIR)

docker-build:
	docker build -t $(APP):latest .

release-snapshot:
	goreleaser release --snapshot --clean

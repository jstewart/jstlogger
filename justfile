# Pinned tool versions; keep them in sync with .github/workflows/ci.yml.
golangci_lint_version := "v2.15.0"
govulncheck_version := "v1.9.0"

default:
    just --list

build:
    go build -gcflags='all=-N -l'

install:
    go install

run: build
    ./jstlogger

test:
    go test ./... -short

format:
    gofmt -w .

vendor:
    go mod tidy && go mod vendor

generate:
    go generate ./...

lint:
    golangci-lint run

vuln:
    govulncheck ./...

# Refresh the embedded DXCC country file, and its copyright notice, from
# AD1C's Big CTY release on country-files.com.
cty:
    #!/usr/bin/env bash
    set -euo pipefail
    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' EXIT
    curl -fsSL -o "$tmp/bigcty.zip" https://www.country-files.com/bigcty/download/bigcty.zip
    unzip -p "$tmp/bigcty.zip" cty.csv > internal/dxcc/cty.csv
    unzip -p "$tmp/bigcty.zip" copyright.txt > internal/dxcc/cty-copyright.txt

# Install the pinned lint and vulnerability tools into $GOBIN.
tools:
    go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@{{golangci_lint_version}}
    go install golang.org/x/vuln/cmd/govulncheck@{{govulncheck_version}}

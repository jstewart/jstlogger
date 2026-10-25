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

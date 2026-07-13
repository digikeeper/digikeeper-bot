bin := "./bin"
cmd := "./cmd"

build:
    go build -o {{ bin }}/digikeeper-bot {{ cmd }}/bot

run:
    go run {{ cmd }}/bot

lint:
    golangci-lint run ./... && go fix -diff ./...

fmt:
    golangci-lint run --fix ./...

fix-diff:
    go fix -diff ./...

fix:
    go fix ./...

test *args:
    go test {{ args }}

unit *args:
    go test {{ args }} ./...

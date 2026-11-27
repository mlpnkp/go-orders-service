.PHONY: run build test cover swagger fmt fmt-check vet comment-guard check

run:
	go run ./cmd/api

build:
	go build ./...

test:
	go test -race ./...

cover:
	go test ./internal/service/... -coverprofile=coverage.out
	go tool cover -func=coverage.out

swagger:
	swag init -g cmd/api/main.go -o docs --parseInternal

fmt:
	gofmt -w cmd internal

vet:
	go vet ./...

fmt-check:
	@test -z "$$(gofmt -l cmd internal)" || (gofmt -l cmd internal; exit 1)

comment-guard:
	@! grep -rnE --include='*.go' '^[[:space:]]*//|[[:space:]]//[[:space:]]' cmd internal | grep -vE '//[[:space:]]*@|//go:'

check: fmt-check comment-guard vet test build

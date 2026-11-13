.PHONY: build frontend test test-go test-web e2e vet

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X 'virtmgr/internal/version.Version=$(VERSION)' -X 'virtmgr/internal/version.Commit=$(COMMIT)' -X 'virtmgr/internal/version.Date=$(BUILD_DATE)'

frontend:
	cd web && npm run build

build: frontend
	CGO_ENABLED=1 go build -ldflags "$(LDFLAGS)" -o bin/virtmgr ./cmd/virtmgr

test-go:
	go test ./...

test-web:
	cd web && npm run typecheck && npm test

test: test-go test-web

vet:
	go vet ./...

e2e:
	cd web && npm run e2e:serial

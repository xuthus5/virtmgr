.PHONY: build frontend test test-go test-web e2e vet

frontend:
	cd web && npm run build

build: frontend
	CGO_ENABLED=1 go build -o bin/virtmgr ./cmd/virtmgr

test-go:
	go test ./...

test-web:
	cd web && npm run typecheck && npm test

test: test-go test-web

vet:
	go vet ./...

e2e:
	cd web && npm run e2e:serial

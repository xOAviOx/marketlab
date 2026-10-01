.PHONY: dev server web test race benchmark build

dev:
	@echo "Run 'make server' and 'make web' in separate terminals."

server:
	go run ./cmd/marketlab

web:
	pnpm --dir web dev

test:
	go test ./...
	pnpm --dir web test -- --run

race:
	go test -race ./...

benchmark:
	go test -run '^$$' -bench . -benchmem ./internal/engine

build:
	sh scripts/build.sh


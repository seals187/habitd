.PHONY: dev test lint check docker-check build docker seed

# Local development without HTTPS. The database lands in ./data.
dev:
	HABITD_DB=./data/habitd.db HABITD_COOKIE_SECURE=false go run ./cmd/habitd

test:
	go test ./...

lint:
	go vet ./...
	go tool staticcheck ./...

# Everything CI checks, after sorting imports and formatting.
check:
	sh tools/ci.sh --fix

# The same without a local Go, in a container.
docker-check:
	sh tools/check.sh

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/habitd ./cmd/habitd

docker:
	docker build -t ghcr.io/seals187/habitd:latest .

seed:
	HABITD_DB=./data/habitd.db go run ./cmd/habitd seed

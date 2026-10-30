.PHONY: dev test lint build docker seed

# Local development without HTTPS. The database lands in ./data.
dev:
	HABITD_DB=./data/habitd.db HABITD_COOKIE_SECURE=false go run ./cmd/habitd

test:
	go test ./...

lint:
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck@latest ./...

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/habitd ./cmd/habitd

docker:
	docker build -t habitd:latest .

seed:
	HABITD_DB=./data/habitd.db go run ./cmd/habitd seed

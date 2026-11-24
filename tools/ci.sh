#!/bin/sh
# The checks every change has to pass. CI runs them as they are; --fix first
# sorts imports and formats the code, as tools/check.sh does locally.
set -e
if [ "$1" = "--fix" ]; then
	go tool goimports -w -local github.com/seals187/habitd cmd internal tools web
fi
unformatted=$(gofmt -l cmd internal tools web)
if [ -n "$unformatted" ]; then
	echo "not formatted, run tools/check.sh:" >&2
	echo "$unformatted" >&2
	exit 1
fi
go vet ./...
go tool staticcheck ./...
sh tools/rules.sh
go test ./...

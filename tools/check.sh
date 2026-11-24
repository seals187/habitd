#!/bin/sh
# Runs the checks of CI in a Go container, for machines without Go:
#   tools/check.sh                 imports, format, vet, staticcheck, rules, tests
#   tools/check.sh go test ./internal/game -run Quota
# Two volumes keep the module and build caches between runs.
set -e
cd "$(dirname "$0")/.."
image=golang:1.27-alpine
if [ $# -eq 0 ]; then
	set -- sh tools/ci.sh --fix
fi
MSYS_NO_PATHCONV=1 exec docker run --rm \
	-v "$(pwd -W 2>/dev/null || pwd):/src" -w /src \
	-v habitd-gomod:/go/pkg/mod -v habitd-gocache:/root/.cache \
	"$image" "$@"

#!/bin/bash
set -uo pipefail
cd /mnt/c/Users/z00587616/Documents/ConnectionAgent/ca-client-tools || exit 1
export GOWORK=off
echo "== go version =="
go version
echo "== gofmt -l =="
gofmt -l $(find . -name '*.go' -not -path './.git/*')
echo "== go build =="
go build ./... || exit 2
echo "== go vet =="
go vet ./... || exit 3
echo "== go test =="
go test ./... -count=1 || exit 4
echo "ALL_LINUX_GATES_PASS"

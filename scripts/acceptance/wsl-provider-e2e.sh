#!/bin/bash
set -uo pipefail
cd /mnt/c/Users/z00587616/Documents/ConnectionAgent/ca-client-tools || exit 1
export GOWORK=off
go build -o /tmp/ca-client-tools-linux ./cmd/ca-client-tools || exit 2
TOKEN=$(bash /mnt/c/Users/z00587616/Documents/ConnectionAgent/.wsl/get-token.sh)
export CA_ACCEPTANCE_PROVIDER_EXE=/tmp/ca-client-tools-linux
export CA_ACCEPTANCE_CLI_EXE=/tmp/ca-client-tools-linux
export CA_ACCEPTANCE_SMF_SBI_ORIGIN=http://127.0.0.1:17777
export CA_ACCEPTANCE_AMF_METRICS_ORIGIN=http://127.0.0.1:19090
export CA_ACCEPTANCE_SMF_METRICS_ORIGIN=http://127.0.0.1:29090
export CA_ACCEPTANCE_QUERY_TOKEN="$TOKEN"
export CA_ACCEPTANCE_SUPI=imsi-999700000000001
export CA_ACCEPTANCE_PSI=1
go test -tags acceptance ./scripts/acceptance/ -count=1 -v 2>&1

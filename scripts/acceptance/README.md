# Windows-native + WSL real-environment acceptance tooling

These scripts support the repeatable acceptance flow described in
[docs/evidence/windows-wsl-acceptance-20261117.md](../../docs/evidence/windows-wsl-acceptance-20261117.md).
They are operator/acceptance tooling, not part of the CLI or the SBI client
libraries, and they are not Agent-callable.

## Topology assumed

- WSL2 runs the real OpenAMSM deployment (`~/env-openamsm`, 11 NFs) and the
  real UERANSIM gNB/UE (`~/env-ueransim`) on the Linux filesystem.
- WSL2 uses **mirrored networking** (`[wsl2] networkingMode=Mirrored`), so
  Windows and WSL share `127.0.0.1`.
- The NFs listen on WSL loopback aliases (`10.30.20.4:7777` SMF SBI,
  `10.30.20.5:9090` AMF metrics, `10.30.20.4:9090` SMF metrics), which Windows
  cannot reach directly.
- `wsl-tcp-forward.sh` exposes exactly those three services on WSL
  `127.0.0.1:17777 / 19090 / 29090`; Windows reaches them on the same loopback
  ports. No Docker, no `netsh portproxy`, no firewall change, no iptables.

## 1. Start the optional WSL forwarder (inside WSL, from Windows)

`wsl-tcp-forward.py` / `wsl-tcp-forward.sh` are an **OPTIONAL,
WSL-mirrored-networking-only acceptance helper — not a product dependency** and
never part of the shipped deployment. The product/client configuration must not
contain any WSL field. Without WSL (or without mirrored networking) do not use
this helper: configure a real remote SBI origin (`CA_OPENAMSM_SMF_SBI_ORIGIN`)
and the real metrics origins instead.

```powershell
wsl.exe -d Ubuntu-26.04 -- bash /mnt/c/<path-to-repo>/scripts/acceptance/wsl-tcp-forward.sh start
wsl.exe -d Ubuntu-26.04 -- bash /mnt/c/<path-to-repo>/scripts/acceptance/wsl-tcp-forward.sh status
```

`status` exits 0 when running and 3 when stopped. The state (PID file and log)
defaults to `~/.ca-acceptance/wsl-forwarder`; the PID changes across restarts,
so always read it from the pidfile or `status`. After a WSL restart, run
`start` again and re-verify.

### Listener security boundary

The forwarder listens on the hardcoded loopback address `127.0.0.1` only:

- there is **no `--bind` option** and `CA_WSL_FORWARD_BIND` is ignored; the
  listen address cannot be configured at all;
- any attempt to bind anything other than `127.0.0.1` (`0.0.0.0`, `::`, `::1`,
  a routed/LAN IPv4 or IPv6 address) is rejected before a listener is created,
  with a clear non-zero exit and no open socket;
- exactly three services/ports are declared (SMF SBI `17777`, AMF metrics
  `19090`, SMF metrics `29090`); there is no fourth listener and no general
  proxy behavior.

The port and backend target env overrides exist only for the isolated
lifecycle test; the listen address itself is never overridable.

### Tests

```powershell
# Offline binding-boundary unit tests (stdlib unittest; Windows or Linux):
cd scripts\acceptance
python -m unittest test_wsl_tcp_forward -v

# Lifecycle test start/status/restart/stop (inside WSL/Linux):
wsl.exe -d Ubuntu-26.04 -u root -- bash /mnt/c/<path-to-repo>/scripts/acceptance/test-wsl-forwarder-lifecycle.sh
```

The lifecycle test uses an isolated `CA_ACCEPTANCE_STATE_DIR` and free
non-default high ports (via `CA_WSL_FORWARD_*_PORT`), so a live acceptance
forwarder on the default ports keeps running untouched; it re-checks that
listener's PID after the test.

## 2. Build the Windows CLI and run the acceptance

```powershell
$env:GOWORK = 'off'   # only if a parent go.work would otherwise mask this module
go build -o .\dist\ca-client-tools.exe .\cmd\ca-client-tools

# Deployment configuration only. Inject the token into the process environment;
# never put it in arguments, files, logs, Git or acceptance documents.
$env:CA_OPENAMSM_QUERY_TOKEN = <deployment query token>

powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\acceptance\windows-e2e.ps1 `
  -Exe .\dist\ca-client-tools.exe `
  -AmfMetricsOrigin http://127.0.0.1:19090 `
  -SmfMetricsOrigin http://127.0.0.1:29090 `
  -SmfSbiOrigin http://127.0.0.1:17777 `
  -Supi imsi-999700000000001 -Psi 1 -Ipv4 10.45.0.2 `
  -WrongIpv4 10.45.0.99 -UnallocatedIpv6 2001:db8:cafe::99 `
  -OutDir .\acceptance-out

Remove-Item Env:CA_OPENAMSM_QUERY_TOKEN
```

The runner asserts real JSON fields, not just exit codes:

- `--version`, `--help` (human output, exit 0);
- AMF UE list: an item with `supi == -Supi`, `pdu_sessions_count >= 1`,
  `pdu_sessions[].psi` containing `-Psi`, and a `pager` object;
- AMF gNB list: an item named `-ExpectedGnbName` (default
  `UERANSIM-gnb-999-70-1`) with `num_connected_ues >= 1`, and a `pager`;
- SMF PDU list: an item with `supi == -Supi` and a `pdu` entry with
  `psi == -Psi`, matching `pdu_state` and (when provided) `ipv4`, and a `pager`;
- the eight SMF context types with the real SUPI + PSI: `data.type` equals the
  requested type, `data.status == AVAILABLE`, `data.key.supi` equals the
  requested SUPI, `data.key.pduSessionId` numerically equals the requested PSI,
  and supplied UE-IP keys are echoed;
- IMSI + PSI (echoed normalized SUPI), IPv4 only (`data.key.ueIpv4` echo),
  SUPI + PSI + IPv4;
- wrong PSI / wrong SUPI: HTTP `404`, cause `SESSION_NOT_FOUND`, exit 4;
- unallocated IPv4 / IPv6: HTTP `409`, cause `KEY_CONFLICT`, exit 4;
- wrong token: HTTP `403`, cause `FORBIDDEN`, exit 4;
- missing token: exit 3, `CONFIGURATION_MISSING`, with the origins pointed at a
  closed loopback port (`http://127.0.0.1:1`) to prove no request was sent;
- unknown context type: exit 2, `INVALID_ARGUMENT`, same no-request proof.

Every JSON case asserts exactly one JSON document on the expected stream and an
empty opposite stream, plus no token or subscriber leakage into error text.
Optional scenarios whose parameters are absent (`-WrongIpv4`,
`-UnallocatedIpv6`, `-Ipv4`) are recorded as `NOT_RUN` with a reason, never as
`PASS`; the summary JSON and console output include PASS/FAIL/NOT_RUN counts.

It stores per-case `stdout`/`stderr`/`exit` files and a `summary.json` in
`-OutDir`. It never prints the token. The runner itself is Windows PowerShell
5.1 compatible.

## 3. Provider process acceptance (P0 + P1)

The resident Provider machine interface is `ca-client-tools provider serve` with
exactly one wire identity: `ca-client-tools.provider/v2`. Legacy
`ca-client-tools.provider/v1` frames are rejected with
`PROTOCOL_VERSION_UNSUPPORTED`; there is no version negotiation, fallback or
compatibility path. `initialize` and `catalog` always report `catalogVersion`
`"2"` and `backendBindingCatalogDigest` = `sha256:` + SHA-256 of the exact
embedded `contracts/provider-bindings/v2/provider-bindings.yaml` bytes.
`initialize` additionally accepts the optional P1 `artifactChannel` bootstrap
from the pinned Host Artifact Store contract
(`ca-host.artifact-store/v1`, ca-agent commit
`2c0b07f2eaff3bc1020bfc6775ef9bf09e3052ae`) and reports its effective limits.

Two Provider binaries participate:

- **production** (no build tags) contains only the eleven typed read bindings.
  It never contains `acceptance.generic.*`, `acceptance.artifact.*`, the fixture
  service keys or the fixture source pin.
- **providerfixture** (`-tags providerfixture`) additionally embeds seven
  acceptance-only genericSBI bindings: `acceptance.generic.echo` (READ),
  `acceptance.generic.mutation` (MUTATION), the artifact-bearing
  `acceptance.generic.binary` / `acceptance.generic.multipart` responses, and
  the P1 Appendix A bindings `acceptance.artifact.binary.up`,
  `acceptance.artifact.binary.down` and
  `acceptance.artifact.multipart.roundtrip` under service key
  `acceptance-artifact`. These bindings exist only in this explicit build and
  are never shipped.

Build both:

```powershell
$env:GOWORK = 'off'
go build -o .\dist\ca-client-tools-provider.exe .\cmd\ca-client-tools
go build -tags providerfixture -o .\dist\ca-client-tools-provider-fixture.exe .\cmd\ca-client-tools
```

### 3.1 Generic and artifact fixture acceptance (no live lab)

`provider_generic_e2e_test.go` starts a deterministic loopback h2c upstream in
the test process, writes a serviceconfig v1 temp file for service key
`acceptance-fixture-sbi`, points the fixture child at it through
`CA_PROVIDER_SERVICE_CONFIG`, and drives real `initialize`/`catalog`/`invoke`/
`cancel` NDJSON frames through the actual `provider serve` child process.

`provider_artifact_e2e_test.go` adds the P1 slice: the same fixture child is
configured with both service keys `acceptance-fixture-sbi` and
`acceptance-artifact`, `initialize` carries the `artifactChannel` bootstrap of an
in-process HTTP/1.1 test double of the Host channel, and the tests exercise
reference validation, channel GET input resolution and POST output staging,
binary and nested/array-pointer multipart round trips, Appendix A
`artifactSlots` descriptors, and the negative matrix (shape/invalid/foreign/
expired references, transfer and staging failures, no output reference on
failure, zero upstream requests for pre-I/O rejects, `NOT_CONFIGURED` without a
channel). Both tests need only the fixture binary:

```powershell
$env:GOWORK = 'off'
$env:CA_ACCEPTANCE_PROVIDER_FIXTURE_EXE = (Resolve-Path .\dist\ca-client-tools-provider-fixture.exe).Path
go test -tags acceptance ./scripts/acceptance/ -run 'TestProviderGeneric|TestProviderArtifact' -count=1 -v
```

The same subset runs through the runner without any live wiring:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\acceptance\windows-provider-e2e.ps1 -GenericOnly
```

Assertions include: initialize catalog version/digest and the effective
artifact limits; catalog partitioning and descriptor hygiene for all eleven
production IDs plus the fixture IDs; nested object+array arguments with exact
integer precision (`9007199254740993`); the trusted `mode=ok` default and fixed
`X-Acceptance-Trusted: fixture-7` header; response JSON preservation; 202 and
204 status preservation; a valid remote `application/problem+json` 404 as a
successful protocol result; malformed or non-problem non-200 as
`REMOTE_RESPONSE_INVALID`; artifact-bearing fixture bindings reported
`NOT_CONFIGURED` and rejected with `BACKEND_OPERATION_UNSUPPORTED`
(`detail.reason: NOT_CONFIGURED`, no extra detail, zero upstream requests) when
no channel is configured; P1 artifactSlots descriptors and binary/multipart
reference round trips with channel GET/POST assertions; `ARTIFACT_REFERENCE_INVALID` /
`ARTIFACT_TRANSFER_FAILED` channel-failure mapping with no emitted reference;
invalid arguments rejected before any upstream request; nested duplicate JSON
keys as `PROTOCOL_FRAME_INVALID`; `CALL_DEADLINE_EXCEEDED`; one terminal
`CALL_CANCELLED` plus `CANCEL_REQUESTED`; and `MUTATION_GATED` with zero
mutation requests.

### 3.2 Live typed acceptance

The live typed tests additionally need the compiled production Provider and the
deployment identity/service origins. The query token belongs only in the
process environment; never pass it as an argument or write it to a file.

```powershell
$env:GOWORK = 'off'
$env:CA_ACCEPTANCE_PROVIDER_EXE = (Resolve-Path .\dist\ca-client-tools-provider.exe).Path
$env:CA_ACCEPTANCE_PROVIDER_FIXTURE_EXE = (Resolve-Path .\dist\ca-client-tools-provider-fixture.exe).Path
$env:CA_ACCEPTANCE_CLI_EXE = $env:CA_ACCEPTANCE_PROVIDER_EXE
$env:CA_ACCEPTANCE_SMF_SBI_ORIGIN = 'http://127.0.0.1:17777'
$env:CA_ACCEPTANCE_AMF_METRICS_ORIGIN = 'http://127.0.0.1:19090'
$env:CA_ACCEPTANCE_SMF_METRICS_ORIGIN = 'http://127.0.0.1:29090'
$env:CA_ACCEPTANCE_QUERY_TOKEN = <deployment query token>
$env:CA_ACCEPTANCE_SUPI = 'imsi-999700000000001'
$env:CA_ACCEPTANCE_PSI = '1'
go test -tags acceptance ./scripts/acceptance/ -count=1 -v
```

The runner builds both binaries and wires the same variables (the token is read
from `CA_ACCEPTANCE_QUERY_TOKEN` only):

```powershell
$env:CA_ACCEPTANCE_QUERY_TOKEN = <deployment query token>
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\acceptance\windows-provider-e2e.ps1 `
  -SmfSbiOrigin http://127.0.0.1:17777 `
  -AmfMetricsOrigin http://127.0.0.1:19090 `
  -SmfMetricsOrigin http://127.0.0.1:29090 `
  -Supi imsi-999700000000001 -Psi 1
```

On Linux/WSL, `wsl-provider-e2e.sh` builds `/tmp/ca-client-tools-linux` and
`/tmp/ca-client-tools-linux-fixture`, exports
`CA_ACCEPTANCE_PROVIDER_FIXTURE_EXE` and keeps the same live wiring.

## 4. HTTP/2 prior-knowledge evidence

The typed SMF adapter is h2c-only: the client's first bytes on the wire are
`PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n`, and the real SMF SBI listener does not
serve HTTP/1.1. The acceptance record contains a loopback-tap capture of that
preface and negative HTTP/1.1 probes; see
[docs/evidence/windows-wsl-acceptance-20261117.md](../../docs/evidence/windows-wsl-acceptance-20261117.md) §5.1.

## Safety notes

- The forwarder binds loopback only and exposes exactly three ports; the bind
  address is hardcoded and cannot be widened.
- Do not disable token authentication for acceptance.
- Do not modify the imported OpenAMSM source snapshot or pristine UERANSIM
  source; use the environment repositories' own CLIs.
- The token must exist only in the test process environment. Do not write it to
  result files, logs, shell history or documentation.

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

## 3. HTTP/2 prior-knowledge evidence

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

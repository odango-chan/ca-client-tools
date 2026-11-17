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

## 1. Start the forwarder (inside WSL, from Windows)

```powershell
wsl.exe -d Ubuntu-26.04 -- bash /mnt/c/<path-to-repo>/scripts/acceptance/wsl-tcp-forward.sh start
wsl.exe -d Ubuntu-26.04 -- bash /mnt/c/<path-to-repo>/scripts/acceptance/wsl-tcp-forward.sh status
```

`status` exits 0 when running and 3 when stopped. The state (PID file and log)
defaults to `~/.ca-acceptance/wsl-forwarder`; the PID changes across restarts,
so always read it from the pidfile or `status`. After a WSL restart, run
`start` again and re-verify.

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

The runner asserts exit codes and JSON status/error codes for:

- `--version`, `--help`;
- AMF UE list, AMF gNB list, SMF PDU list;
- the eight SMF context types with the real SUPI + PSI;
- IMSI + PSI, IPv4 only, SUPI + PSI + IPv4;
- wrong PSI, wrong SUPI, unallocated IPv4/IPv6, wrong token, missing token,
  unknown context type.

It stores per-case `stdout`/`stderr`/`exit` files and a `summary.json` in
`-OutDir`. It never prints the token.

## 3. HTTP/2 prior-knowledge evidence

The typed SMF adapter is h2c-only: the client's first bytes on the wire are
`PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n`, and the real SMF SBI listener does not
serve HTTP/1.1. The acceptance record contains a loopback-tap capture of that
preface and negative HTTP/1.1 probes; see
[docs/evidence/windows-wsl-acceptance-20261117.md](../../docs/evidence/windows-wsl-acceptance-20261117.md) §5.1.

## Safety notes

- The forwarder binds loopback only and exposes exactly three ports.
- Do not disable token authentication for acceptance.
- Do not modify the imported OpenAMSM source snapshot or pristine UERANSIM
  source; use the environment repositories' own CLIs.
- The token must exist only in the test process environment. Do not write it to
  result files, logs, shell history or documentation.

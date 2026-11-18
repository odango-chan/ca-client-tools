<#
.SYNOPSIS
    Build both Provider binaries and run the P0 Provider acceptance tests.

.DESCRIPTION
    Builds:
      * the production Provider (no build tags; eleven typed read bindings only)
      * the providerfixture Provider (-tags providerfixture; adds the four
        acceptance-only genericSBI bindings acceptance.generic.echo,
        acceptance.generic.mutation, acceptance.generic.binary and
        acceptance.generic.multipart)

    The generic fixture acceptance (-GenericOnly, or the TestProviderGeneric
    subset of the full run) only needs the fixture binary and runs against an
    in-process deterministic loopback h2c upstream. It never contacts the live
    OpenAMSM lab.

    The live typed tests need the CA_ACCEPTANCE_* live wiring. Pass the origins
    and identity via parameters or the corresponding environment variables. The
    query token is never a parameter: set it only in the process environment
    (CA_ACCEPTANCE_QUERY_TOKEN), as documented in scripts/acceptance/README.md.

.EXAMPLE
    # Generic fixture acceptance only (no live lab, no token):
    powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\acceptance\windows-provider-e2e.ps1 -GenericOnly

.EXAMPLE
    # Full Provider acceptance against the live lab:
    $env:CA_ACCEPTANCE_QUERY_TOKEN = '<deployment query token>'
    .\scripts\acceptance\windows-provider-e2e.ps1 `
        -SmfSbiOrigin http://127.0.0.1:17777 `
        -AmfMetricsOrigin http://127.0.0.1:19090 `
        -SmfMetricsOrigin http://127.0.0.1:29090 `
        -Supi imsi-999700000000001 -Psi 1
#>
[CmdletBinding()]
param(
    [string]$OutDir = 'dist',
    [string]$ProviderExe = '',
    [string]$FixtureExe = '',
    [string]$SmfSbiOrigin = $env:CA_ACCEPTANCE_SMF_SBI_ORIGIN,
    [string]$AmfMetricsOrigin = $env:CA_ACCEPTANCE_AMF_METRICS_ORIGIN,
    [string]$SmfMetricsOrigin = $env:CA_ACCEPTANCE_SMF_METRICS_ORIGIN,
    [string]$Supi = $env:CA_ACCEPTANCE_SUPI,
    [string]$Psi = $env:CA_ACCEPTANCE_PSI,
    [switch]$GenericOnly
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
Push-Location $repoRoot
try {
    $env:GOWORK = 'off'

    if ([System.IO.Path]::IsPathRooted($OutDir)) {
        $outPath = $OutDir
    } else {
        $outPath = Join-Path $repoRoot $OutDir
    }
    New-Item -ItemType Directory -Force -Path $outPath | Out-Null
    if ([string]::IsNullOrWhiteSpace($ProviderExe)) {
        $ProviderExe = Join-Path $outPath 'ca-client-tools-provider.exe'
    }
    if ([string]::IsNullOrWhiteSpace($FixtureExe)) {
        $FixtureExe = Join-Path $outPath 'ca-client-tools-provider-fixture.exe'
    }

    Write-Host "Building production Provider -> $ProviderExe"
    & go build -o $ProviderExe ./cmd/ca-client-tools
    if ($LASTEXITCODE -ne 0) { throw "production Provider build failed (exit $LASTEXITCODE)" }

    Write-Host "Building providerfixture Provider -> $FixtureExe"
    & go build -tags providerfixture -o $FixtureExe ./cmd/ca-client-tools
    if ($LASTEXITCODE -ne 0) { throw "providerfixture Provider build failed (exit $LASTEXITCODE)" }

    $env:CA_ACCEPTANCE_PROVIDER_EXE = (Resolve-Path $ProviderExe).Path
    $env:CA_ACCEPTANCE_PROVIDER_FIXTURE_EXE = (Resolve-Path $FixtureExe).Path
    $env:CA_ACCEPTANCE_CLI_EXE = $env:CA_ACCEPTANCE_PROVIDER_EXE

    if ($GenericOnly) {
        Write-Host 'Running generic fixture acceptance only (deterministic loopback upstream; no live lab).'
        & go test -tags acceptance ./scripts/acceptance/ -run TestProviderGeneric -count=1 -v
        exit $LASTEXITCODE
    }

    # Live typing requires the deployment identity and service origins. The
    # token is read from the environment only and is never logged here.
    if ([string]::IsNullOrWhiteSpace($env:CA_ACCEPTANCE_QUERY_TOKEN)) {
        throw 'CA_ACCEPTANCE_QUERY_TOKEN must be set in the process environment for live acceptance.'
    }
    if ([string]::IsNullOrWhiteSpace($SmfSbiOrigin) -or
        [string]::IsNullOrWhiteSpace($AmfMetricsOrigin) -or
        [string]::IsNullOrWhiteSpace($SmfMetricsOrigin)) {
        throw 'Pass -SmfSbiOrigin/-AmfMetricsOrigin/-SmfMetricsOrigin or set the CA_ACCEPTANCE_* origins.'
    }
    if ([string]::IsNullOrWhiteSpace($Supi) -or [string]::IsNullOrWhiteSpace($Psi)) {
        throw 'Pass -Supi/-Psi or set CA_ACCEPTANCE_SUPI/CA_ACCEPTANCE_PSI.'
    }
    $env:CA_ACCEPTANCE_SMF_SBI_ORIGIN = $SmfSbiOrigin
    $env:CA_ACCEPTANCE_AMF_METRICS_ORIGIN = $AmfMetricsOrigin
    $env:CA_ACCEPTANCE_SMF_METRICS_ORIGIN = $SmfMetricsOrigin
    $env:CA_ACCEPTANCE_SUPI = $Supi
    $env:CA_ACCEPTANCE_PSI = $Psi

    Write-Host 'Running full Provider acceptance (live typed tests + generic fixture tests).'
    & go test -tags acceptance ./scripts/acceptance/ -count=1 -v
    exit $LASTEXITCODE
}
finally {
    Pop-Location
}

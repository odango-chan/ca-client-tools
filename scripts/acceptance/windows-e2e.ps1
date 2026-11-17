# Windows-native OpenAMSM CLI acceptance runner.
#
# Runs the compiled Windows ca-client-tools.exe against a real OpenAMSM
# deployment reached through operator-provided trusted origins. All subscriber
# values and origins are parameters; the query token is read only from this
# process environment and is never written to stdout, stderr, result files or
# the summary.
#
# This runner executes on Windows PowerShell 5.1 (default on Windows) and needs
# no external modules. Every case spawns one CLI process with a redirected
# stdin/stdout/stderr and one inherited environment, then asserts the exit code
# and (optionally) the JSON `data.status` / `error.code`.
#
# Usage (run from the repository root after `go build`):
#   $env:CA_OPENAMSM_QUERY_TOKEN = <deployment query token, process env only>
#   powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\acceptance\windows-e2e.ps1 `
#     -Exe .\dist\ca-client-tools.exe `
#     -AmfMetricsOrigin http://127.0.0.1:19090 `
#     -SmfMetricsOrigin http://127.0.0.1:29090 `
#     -SmfSbiOrigin http://127.0.0.1:17777 `
#     -Supi imsi-999700000000001 -Psi 1 -Ipv4 10.45.0.2 `
#     -WrongIpv4 10.45.0.99 -UnallocatedIpv6 2001:db8:cafe::99 `
#     -OutDir .\acceptance-out
#
# Exit code 0 when every asserted expectation matched, 1 on mismatch, 3 when
# the token is missing from the process environment.

param(
    [Parameter(Mandatory = $true)][string]$Exe,
    [Parameter(Mandatory = $true)][string]$AmfMetricsOrigin,
    [Parameter(Mandatory = $true)][string]$SmfMetricsOrigin,
    [Parameter(Mandatory = $true)][string]$SmfSbiOrigin,
    [Parameter(Mandatory = $true)][string]$Supi,
    [Parameter(Mandatory = $false)][int]$Psi = 1,
    [Parameter(Mandatory = $false)][string]$Ipv4 = "",
    [Parameter(Mandatory = $false)][string]$WrongIpv4 = "",
    [Parameter(Mandatory = $false)][string]$UnallocatedIpv6 = "",
    [Parameter(Mandatory = $false)][string]$OutDir = ".\acceptance-out"
)

$ErrorActionPreference = "Stop"
$exePath = (Resolve-Path $Exe).Path
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
$OutDir = (Resolve-Path $OutDir).Path
$script:Failures = @()
$script:Cases = @()

function Invoke-Cli {
    param(
        [string[]]$Arguments,
        [hashtable]$Environment,
        [string]$StdinText = $null,
        [string[]]$RemoveEnvironment = @()
    )
    $inFile = [System.IO.Path]::GetTempFileName()
    $outFile = [System.IO.Path]::GetTempFileName()
    $errFile = [System.IO.Path]::GetTempFileName()
    [System.IO.File]::WriteAllText($inFile, $(if ($null -ne $StdinText) { $StdinText } else { "" }), (New-Object System.Text.UTF8Encoding($false)))
    $savedEnvironment = @{}
    try {
        foreach ($name in $RemoveEnvironment) {
            $savedEnvironment[$name] = [Environment]::GetEnvironmentVariable($name)
            [Environment]::SetEnvironmentVariable($name, $null)
        }
        foreach ($name in $Environment.Keys) {
            if (-not $savedEnvironment.ContainsKey($name)) {
                $savedEnvironment[$name] = [Environment]::GetEnvironmentVariable($name)
            }
            [Environment]::SetEnvironmentVariable($name, $Environment[$name])
        }
        $process = Start-Process -FilePath $exePath -ArgumentList $Arguments -NoNewWindow -PassThru -Wait `
            -RedirectStandardInput $inFile -RedirectStandardOutput $outFile -RedirectStandardError $errFile
        $exitCode = $process.ExitCode
        $stdout = [System.IO.File]::ReadAllText($outFile)
        $stderr = [System.IO.File]::ReadAllText($errFile)
    } finally {
        foreach ($name in $savedEnvironment.Keys) {
            [Environment]::SetEnvironmentVariable($name, $savedEnvironment[$name])
        }
        Remove-Item $inFile, $outFile, $errFile -ErrorAction SilentlyContinue
    }
    return [pscustomobject]@{
        ExitCode = $exitCode
        Stdout   = $stdout
        Stderr   = $stderr
    }
}

function Save-Result {
    param([string]$Name, $Result)
    $safe = $Name -replace '[^A-Za-z0-9._-]', '_'
    Set-Content -Path (Join-Path $OutDir "$safe.stdout") -Value $Result.Stdout -NoNewline -Encoding utf8
    Set-Content -Path (Join-Path $OutDir "$safe.stderr") -Value $Result.Stderr -NoNewline -Encoding utf8
    Set-Content -Path (Join-Path $OutDir "$safe.exit") -Value "$($Result.ExitCode)" -Encoding ascii
}

function Add-Case {
    param(
        [string]$Name,
        [string]$Command,
        $Result,
        [int]$ExpectedExit,
        [string]$ExpectedStatus = "",
        [string]$ExpectedCode = ""
    )
    $status = $null
    $code = $null
    if ($Result.Stdout.Trim() -ne "") {
        try { $status = ($Result.Stdout | ConvertFrom-Json).data.status } catch { }
    }
    if ($Result.Stderr.Trim() -ne "") {
        try { $code = ($Result.Stderr | ConvertFrom-Json).error.code } catch { }
    }
    $ok = ($Result.ExitCode -eq $ExpectedExit)
    if ($ok -and $ExpectedStatus -ne "") { $ok = ($status -eq $ExpectedStatus) }
    if ($ok -and $ExpectedCode -ne "") { $ok = ($code -eq $ExpectedCode) }
    if (-not $ok) {
        $script:Failures += "$Name (exit=$($Result.ExitCode) want=$ExpectedExit status=$status code=$code)"
    }
    $record = [pscustomobject]@{
        Case       = $Name
        Command    = $Command
        ExitCode   = $Result.ExitCode
        WantExit   = $ExpectedExit
        Status     = "$status"
        WantStatus = $ExpectedStatus
        ErrorCode  = "$code"
        Result     = if ($ok) { "PASS" } else { "FAIL" }
    }
    $script:Cases += $record
    Save-Result -Name $Name -Result $Result
    return $record
}

$common = @{
    CA_OPENAMSM_AMF_METRICS_ORIGIN = $AmfMetricsOrigin
    CA_OPENAMSM_SMF_METRICS_ORIGIN = $SmfMetricsOrigin
    CA_OPENAMSM_SMF_SBI_ORIGIN     = $SmfSbiOrigin
}
$token = [Environment]::GetEnvironmentVariable("CA_OPENAMSM_QUERY_TOKEN")
if ([string]::IsNullOrEmpty($token)) {
    Write-Error "CA_OPENAMSM_QUERY_TOKEN must be injected into this process environment (it is never taken from a file/argument)."
    exit 3
}
$withToken = $common.Clone()
$withToken["CA_OPENAMSM_QUERY_TOKEN"] = $token

# 1. CLI process contract
$version = Invoke-Cli -Arguments @("--version") -Environment @{}
Add-Case -Name "version" -Command "ca-client-tools --version" -Result $version -ExpectedExit 0 | Out-Null
$help = Invoke-Cli -Arguments @("--help") -Environment @{}
Add-Case -Name "help" -Command "ca-client-tools --help" -Result $help -ExpectedExit 0 | Out-Null

# 2. Metrics list surfaces (plain HTTP/1.1 clients, separate from SBI transport)
$ue = Invoke-Cli -Arguments @("amf", "ue", "list", "--page", "0", "--page-size", "100") -Environment $common
Add-Case -Name "amf-ue-list" -Command "amf ue list" -Result $ue -ExpectedExit 0 | Out-Null
$gnb = Invoke-Cli -Arguments @("amf", "gnb", "list", "--page", "0", "--page-size", "100") -Environment $common
Add-Case -Name "amf-gnb-list" -Command "amf gnb list" -Result $gnb -ExpectedExit 0 | Out-Null
$pdu = Invoke-Cli -Arguments @("smf", "pdu", "list", "--page", "0", "--page-size", "100") -Environment $common
Add-Case -Name "smf-pdu-list" -Command "smf pdu list" -Result $pdu -ExpectedExit 0 | Out-Null

# 3. Eight SMF context queries with the real SUPI + PSI
$types = @("session", "policy", "up-node", "signaling-session", "ip-address", "up-selection", "charging", "session-auth")
$identityBody = (@{ supi = $Supi; pduSessionId = $Psi } | ConvertTo-Json -Compress)
foreach ($type in $types) {
    $result = Invoke-Cli -Arguments @("smf", "context", "query", "--type", $type) -Environment $withToken -StdinText $identityBody
    Add-Case -Name "context-$type" -Command "smf context query --type $type" -Result $result -ExpectedExit 0 -ExpectedStatus "AVAILABLE" | Out-Null
}

# 4. Alternative real lookup shapes
$imsi = $Supi -replace '^imsi-', ''
$imsiBody = (@{ imsi = $imsi; pduSessionId = $Psi } | ConvertTo-Json -Compress)
$r = Invoke-Cli -Arguments @("smf", "context", "query", "--type", "session") -Environment $withToken -StdinText $imsiBody
Add-Case -Name "context-imsi-psi" -Command "imsi + PSI" -Result $r -ExpectedExit 0 -ExpectedStatus "AVAILABLE" | Out-Null

if ($Ipv4 -ne "") {
    $ipv4Body = (@{ supi = $Supi; pduSessionId = $Psi; ueIpv4 = $Ipv4 } | ConvertTo-Json -Compress)
    $r = Invoke-Cli -Arguments @("smf", "context", "query", "--type", "session") -Environment $withToken -StdinText $ipv4Body
    Add-Case -Name "context-supi-psi-ipv4" -Command "SUPI + PSI + IPv4" -Result $r -ExpectedExit 0 -ExpectedStatus "AVAILABLE" | Out-Null

    $ipv4OnlyBody = (@{ ueIpv4 = $Ipv4 } | ConvertTo-Json -Compress)
    $r = Invoke-Cli -Arguments @("smf", "context", "query", "--type", "session") -Environment $withToken -StdinText $ipv4OnlyBody
    Add-Case -Name "context-ipv4-only" -Command "IPv4 only" -Result $r -ExpectedExit 0 -ExpectedStatus "AVAILABLE" | Out-Null
}

# 5. Negative lookups: real 404 SESSION_NOT_FOUND preserved as remote error (exit 4)
$wrongPsiBody = (@{ supi = $Supi; pduSessionId = ($Psi + 1) } | ConvertTo-Json -Compress)
$r = Invoke-Cli -Arguments @("smf", "context", "query", "--type", "session") -Environment $withToken -StdinText $wrongPsiBody
Add-Case -Name "negative-wrong-psi" -Command "wrong PSI" -Result $r -ExpectedExit 4 -ExpectedCode "OPENAMSM_HTTP_ERROR" | Out-Null

$wrongSupiBody = (@{ supi = "imsi-999700000000099"; pduSessionId = $Psi } | ConvertTo-Json -Compress)
$r = Invoke-Cli -Arguments @("smf", "context", "query", "--type", "session") -Environment $withToken -StdinText $wrongSupiBody
Add-Case -Name "negative-wrong-supi" -Command "wrong SUPI" -Result $r -ExpectedExit 4 -ExpectedCode "OPENAMSM_HTTP_ERROR" | Out-Null

if ($WrongIpv4 -ne "") {
    $unallocatedIpv4Body = (@{ supi = $Supi; pduSessionId = $Psi; ueIpv4 = $WrongIpv4 } | ConvertTo-Json -Compress)
    $r = Invoke-Cli -Arguments @("smf", "context", "query", "--type", "session") -Environment $withToken -StdinText $unallocatedIpv4Body
    Add-Case -Name "negative-unallocated-ipv4" -Command "unallocated IPv4" -Result $r -ExpectedExit 4 -ExpectedCode "OPENAMSM_HTTP_ERROR" | Out-Null
}

if ($UnallocatedIpv6 -ne "") {
    $unallocatedIpv6Body = (@{ supi = $Supi; pduSessionId = $Psi; ueIpv6 = $UnallocatedIpv6 } | ConvertTo-Json -Compress)
    $r = Invoke-Cli -Arguments @("smf", "context", "query", "--type", "session") -Environment $withToken -StdinText $unallocatedIpv6Body
    Add-Case -Name "negative-unallocated-ipv6" -Command "unallocated IPv6" -Result $r -ExpectedExit 4 -ExpectedCode "OPENAMSM_HTTP_ERROR" | Out-Null
}

# 6. Token rejection: wrong token reaches the real source as 403, missing token is local config
$wrongToken = $common.Clone()
$wrongToken["CA_OPENAMSM_QUERY_TOKEN"] = "definitely-wrong-token"
$r = Invoke-Cli -Arguments @("smf", "context", "query", "--type", "session") -Environment $wrongToken -StdinText $identityBody
Add-Case -Name "negative-wrong-token" -Command "wrong token" -Result $r -ExpectedExit 4 -ExpectedCode "OPENAMSM_HTTP_ERROR" | Out-Null

$r = Invoke-Cli -Arguments @("smf", "context", "query", "--type", "session") -Environment $common -RemoveEnvironment @("CA_OPENAMSM_QUERY_TOKEN") -StdinText $identityBody
Add-Case -Name "negative-missing-token" -Command "missing token" -Result $r -ExpectedExit 3 -ExpectedCode "CONFIGURATION_MISSING" | Out-Null

# 7. Invalid input is rejected locally before any network access
$r = Invoke-Cli -Arguments @("smf", "context", "query", "--type", "not-a-context") -Environment $withToken -StdinText $identityBody
Add-Case -Name "invalid-context-type" -Command "unknown --type" -Result $r -ExpectedExit 2 -ExpectedCode "INVALID_ARGUMENT" | Out-Null

$script:Cases | Format-Table -AutoSize | Out-String | Write-Host
$summary = [pscustomobject]@{
    Exe              = $exePath
    Supi             = $Supi
    Psi              = $Psi
    Ipv4             = $Ipv4
    Cases            = $script:Cases
    Failures         = $script:Failures
    Result           = if ($script:Failures.Count -eq 0) { "PASS" } else { "FAIL" }
    GeneratedAtLocal = (Get-Date).ToString("o")
}
$summary | ConvertTo-Json -Depth 5 | Set-Content -Path (Join-Path $OutDir "summary.json") -Encoding utf8
if ($script:Failures.Count -gt 0) {
    Write-Host "FAILURES:" ($script:Failures -join "; ")
    exit 1
}
Write-Host "ALL CASES PASS ($($script:Cases.Count))"
exit 0

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
# stdin/stdout/stderr and one inherited environment, then asserts real JSON
# fields, not just exit codes:
#
#   * success: exactly one JSON `{"data": ...}` document on stdout, empty
#     stderr, exit code, `data.type`, `data.status`, `data.key` identity
#     binding and list-item fields;
#   * failure: exactly one JSON `{"error": ...}` document on stderr, empty
#     stdout, exit code, `error.code`, numeric `error.httpStatus` and
#     `error.cause` where the source defines them, and no token/subscriber
#     leakage into the error text.
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
# Exit codes: 0 when every asserted expectation matched (NOT_RUN optional
# scenarios are reported but do not fail the run), 1 on any mismatch, 2 when a
# required runner parameter is missing/invalid, 3 when the token is missing
# from the process environment.
#
# -SelfTest runs the runner's own assertion engine against synthetic JSON
# fixtures without needing the token, origins or a live deployment:
#   powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\acceptance\windows-e2e.ps1 -SelfTest

param(
    [string]$Exe = "",
    [string]$AmfMetricsOrigin = "",
    [string]$SmfMetricsOrigin = "",
    [string]$SmfSbiOrigin = "",
    [string]$Supi = "",
    [int]$Psi = 1,
    [string]$Ipv4 = "",
    [string]$WrongIpv4 = "",
    [string]$UnallocatedIpv6 = "",
    [string]$WrongSupi = "imsi-999700000000099",
    [string]$ExpectedGnbName = "UERANSIM-gnb-999-70-1",
    [string]$ExpectedPduState = "active",
    [string]$OutDir = ".\acceptance-out",
    [switch]$SelfTest
)

$ErrorActionPreference = "Stop"

# Runner-level failure: write to stderr and exit with the documented code.
# (Write-Error would become a terminating error under ErrorActionPreference and
# abort with exit 1 instead of the intended 2/3.)
function Fail-Runner {
    param([string]$Message, [int]$ExitCode)
    [Console]::Error.WriteLine($Message)
    exit $ExitCode
}

$script:Failures = @()
$script:Cases = @()
$script:Token = ""
$script:SensitiveValues = @()
$script:SelfTestFailures = 0
$script:ClosedLoopbackOrigin = ""

# ---------------------------------------------------------------------------
# Strict JSON document parsing (exactly one document, nothing after it)
# ---------------------------------------------------------------------------
function Get-JsonDocument {
    param([string]$Text)

    if ([string]::IsNullOrWhiteSpace($Text)) {
        return [pscustomobject]@{ Valid = $false; Reason = "stream is empty"; Value = $null }
    }
    $length = $Text.Length
    $start = 0
    while ($start -lt $length -and [char]::IsWhiteSpace($Text[$start])) { $start++ }
    if ($start -ge $length) {
        return [pscustomobject]@{ Valid = $false; Reason = "stream is empty"; Value = $null }
    }
    if ($Text[$start] -ne '{') {
        return [pscustomobject]@{ Valid = $false; Reason = "stream does not start with a JSON object"; Value = $null }
    }

    $depth = 0
    $inString = $false
    $escaped = $false
    $end = -1
    for ($index = $start; $index -lt $length; $index++) {
        $ch = $Text[$index]
        if ($inString) {
            if ($escaped) { $escaped = $false }
            elseif ($ch -eq '\') { $escaped = $true }
            elseif ($ch -eq '"') { $inString = $false }
            continue
        }
        if ($ch -eq '"') { $inString = $true; continue }
        if ($ch -eq '{') {
            $depth++
        } elseif ($ch -eq '}') {
            $depth--
            if ($depth -lt 0) {
                return [pscustomobject]@{ Valid = $false; Reason = "unbalanced JSON braces"; Value = $null }
            }
            if ($depth -eq 0) { $end = $index; break }
        }
    }
    if ($end -lt 0) {
        return [pscustomobject]@{ Valid = $false; Reason = "unterminated JSON object"; Value = $null }
    }
    $trailing = $Text.Substring($end + 1)
    if ($trailing.Trim() -ne "") {
        return [pscustomobject]@{ Valid = $false; Reason = "more than one JSON document (trailing data on the stream)"; Value = $null }
    }
    try {
        $value = $Text.Substring($start, $end - $start + 1) | ConvertFrom-Json
    } catch {
        return [pscustomobject]@{ Valid = $false; Reason = "invalid JSON: $($_.Exception.Message)"; Value = $null }
    }
    if ($null -eq $value) {
        return [pscustomobject]@{ Valid = $false; Reason = "JSON document is null"; Value = $null }
    }
    return [pscustomobject]@{ Valid = $true; Reason = ""; Value = $value }
}

function Test-JsonProperty {
    param($Object, [string]$Name)
    if ($null -eq $Object) { return $false }
    return ($null -ne $Object.PSObject.Properties[$Name])
}

function Test-SensitiveLeakage {
    param([string]$Text, [string[]]$Values)
    if ([string]::IsNullOrEmpty($Text)) { return $null }
    foreach ($value in $Values) {
        if ([string]::IsNullOrEmpty($value)) { continue }
        if ($Text.Contains($value)) {
            return "sensitive value leaked into the error stream"
        }
    }
    return $null
}

function Get-ItemByField {
    param($Items, [string]$Field, [string]$Value)
    foreach ($item in @($Items)) {
        if ($null -eq $item) { continue }
        if ("$($item.$Field)" -eq $Value) { return $item }
    }
    return $null
}

# ---------------------------------------------------------------------------
# CLI process invocation
# ---------------------------------------------------------------------------
function ConvertTo-ProcessArguments {
    param([string[]]$Arguments)
    $parts = @()
    foreach ($argument in $Arguments) {
        if ($argument -match '[\s"]') {
            $escaped = $argument -replace '(\\*)"', '$1$1\"' -replace '(\\+)$', '$1$1'
            $parts += ('"' + $escaped + '"')
        } else {
            $parts += $argument
        }
    }
    return ($parts -join ' ')
}

function Invoke-Cli {
    param(
        [string[]]$Arguments,
        [hashtable]$Environment = @{},
        [string]$StdinText = $null,
        [string[]]$RemoveEnvironment = @(),
        [int]$TimeoutSeconds = 60
    )
    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    $stdinValue = ""
    if ($null -ne $StdinText) { $stdinValue = $StdinText }
    $savedEnvironment = @{}
    $timedOut = $false
    try {
        foreach ($name in $RemoveEnvironment) {
            $savedEnvironment[$name] = [Environment]::GetEnvironmentVariable($name)
            [Environment]::SetEnvironmentVariable($name, $null)
        }
        foreach ($name in $Environment.Keys) {
            if (-not $savedEnvironment.ContainsKey($name)) {
                $savedEnvironment[$name] = [Environment]::GetEnvironmentVariable($name)
            }
            [Environment]::SetEnvironmentVariable($name, [string]$Environment[$name])
        }
        # Direct System.Diagnostics.Process (not Start-Process) is used because
        # Windows PowerShell 5.1 loses Process.ExitCode when a redirected
        # non-blocking Start-Process is followed by WaitForExit(timeout).
        $startInfo = New-Object System.Diagnostics.ProcessStartInfo
        $startInfo.FileName = $script:ExePath
        $startInfo.Arguments = ConvertTo-ProcessArguments -Arguments $Arguments
        $startInfo.UseShellExecute = $false
        $startInfo.CreateNoWindow = $true
        $startInfo.RedirectStandardInput = $true
        $startInfo.RedirectStandardOutput = $true
        $startInfo.RedirectStandardError = $true
        $startInfo.StandardOutputEncoding = $utf8NoBom
        $startInfo.StandardErrorEncoding = $utf8NoBom
        $process = New-Object System.Diagnostics.Process
        $process.StartInfo = $startInfo
        $null = $process.Start()
        # The child may legitimately exit before reading stdin for local
        # argument/configuration failures; a broken pipe there is a valid
        # outcome, not a runner failure.
        try {
            $process.StandardInput.Write($stdinValue)
        } catch {
        } finally {
            try { $process.StandardInput.Close() } catch { }
        }
        $stdoutTask = $process.StandardOutput.ReadToEndAsync()
        $stderrTask = $process.StandardError.ReadToEndAsync()
        if (-not $process.WaitForExit($TimeoutSeconds * 1000)) {
            $timedOut = $true
            try { $process.Kill() } catch { }
            $process.WaitForExit()
        }
        $stdout = $stdoutTask.GetAwaiter().GetResult()
        $stderr = $stderrTask.GetAwaiter().GetResult()
        $exitCode = $process.ExitCode
    } finally {
        foreach ($name in $savedEnvironment.Keys) {
            [Environment]::SetEnvironmentVariable($name, $savedEnvironment[$name])
        }
    }
    return [pscustomobject]@{
        ExitCode = $exitCode
        Stdout   = $stdout
        Stderr   = $stderr
        TimedOut = $timedOut
    }
}

function Save-Result {
    param([string]$Name, $Result)
    $safe = $Name -replace '[^A-Za-z0-9._-]', '_'
    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText((Join-Path $OutDir "$safe.stdout"), "$($Result.Stdout)", $utf8NoBom)
    [System.IO.File]::WriteAllText((Join-Path $OutDir "$safe.stderr"), "$($Result.Stderr)", $utf8NoBom)
    [System.IO.File]::WriteAllText((Join-Path $OutDir "$safe.exit"), [string]$Result.ExitCode, $utf8NoBom)
}

# ---------------------------------------------------------------------------
# Case assertion engine
# ---------------------------------------------------------------------------
function Add-Case {
    param(
        [string]$Name,
        [string]$Command,
        $Result,
        [int]$ExpectedExit,
        [string]$ExpectedStatus = "",
        [string]$ExpectedCode = "",
        [int]$ExpectedHttpStatus = -1,
        [string]$ExpectedCause = "",
        [string]$ExpectedType = "",
        [switch]$HumanOutput,
        [switch]$ExpectNoHttpStatus,
        [switch]$ExpectNoCause,
        [scriptblock]$Check = $null,
        [string]$NotRunReason = "",
        [switch]$SkipSave
    )

    if ($NotRunReason -ne "") {
        $notRun = [pscustomobject]@{
            Case       = $Name
            Command    = $Command
            ExitCode   = $null
            WantExit   = $ExpectedExit
            Status     = ""
            WantStatus = $ExpectedStatus
            ErrorCode  = ""
            WantCode   = $ExpectedCode
            HttpStatus = ""
            WantHttp   = ""
            Cause      = ""
            WantCause  = ""
            Result     = "NOT_RUN"
            Reason     = $NotRunReason
        }
        $script:Cases += $notRun
        return $notRun
    }

    $problems = New-Object System.Collections.ArrayList
    $data = $null
    $errorObject = $null
    $stdoutText = "$($Result.Stdout)"
    $stderrText = "$($Result.Stderr)"
    $actualStatus = ""
    $actualCode = ""
    $actualHttpStatus = ""
    $actualCause = ""

    if ($Result.TimedOut) {
        [void]$problems.Add("CLI invocation timed out")
    }

    if ($HumanOutput) {
        if ($Result.ExitCode -ne $ExpectedExit) {
            [void]$problems.Add("exit=$($Result.ExitCode) want=$ExpectedExit")
        }
        if ($stdoutText.Trim() -eq "") {
            [void]$problems.Add("human-readable stdout is empty")
        }
        if ($stderrText.Trim() -ne "") {
            [void]$problems.Add("stderr must be empty")
        }
    } else {
        if ($ExpectedExit -eq 0) {
            $document = Get-JsonDocument -Text $stdoutText
            if (-not $document.Valid) {
                [void]$problems.Add("stdout JSON: $($document.Reason)")
            } else {
                $envelope = $document.Value
                if (-not (Test-JsonProperty $envelope "data")) {
                    [void]$problems.Add("stdout envelope has no `"data`" object")
                } else {
                    $data = $envelope.data
                }
            }
            if ($stderrText.Trim() -ne "") {
                [void]$problems.Add("stderr must be empty on success")
            }
        } else {
            $document = Get-JsonDocument -Text $stderrText
            if (-not $document.Valid) {
                [void]$problems.Add("stderr JSON: $($document.Reason)")
            } else {
                $envelope = $document.Value
                if (-not (Test-JsonProperty $envelope "error")) {
                    [void]$problems.Add("stderr envelope has no `"error`" object")
                } else {
                    $errorObject = $envelope.error
                }
            }
            if ($stdoutText.Trim() -ne "") {
                [void]$problems.Add("stdout must be empty on an operational failure")
            }
        }
        if ($Result.ExitCode -ne $ExpectedExit) {
            [void]$problems.Add("exit=$($Result.ExitCode) want=$ExpectedExit")
        }

        if ($null -ne $data) {
            if ($ExpectedStatus -ne "") {
                $actualStatus = "$($data.status)"
                if ($actualStatus -ne $ExpectedStatus) {
                    [void]$problems.Add("data.status='$actualStatus' want='$ExpectedStatus'")
                }
            }
            if ($ExpectedType -ne "") {
                $actualType = "$($data.type)"
                if ($actualType -ne $ExpectedType) {
                    [void]$problems.Add("data.type='$actualType' want='$ExpectedType'")
                }
            }
        }
        if ($null -ne $errorObject) {
            if ($ExpectedCode -ne "") {
                $actualCode = "$($errorObject.code)"
                if ($actualCode -ne $ExpectedCode) {
                    [void]$problems.Add("error.code='$actualCode' want='$ExpectedCode'")
                }
            }
            if ($ExpectedHttpStatus -ge 0) {
                if (-not (Test-JsonProperty $errorObject "httpStatus")) {
                    [void]$problems.Add("error.httpStatus is missing (want $ExpectedHttpStatus)")
                } else {
                    $statusValue = $errorObject.httpStatus
                    $actualHttpStatus = "$statusValue"
                    if (-not ($statusValue -is [int] -or $statusValue -is [long] -or $statusValue -is [double])) {
                        [void]$problems.Add("error.httpStatus is not numeric: '$statusValue'")
                    } elseif ([int]$statusValue -ne $ExpectedHttpStatus) {
                        [void]$problems.Add("error.httpStatus=$statusValue want=$ExpectedHttpStatus")
                    }
                }
            }
            if ($ExpectNoHttpStatus -and (Test-JsonProperty $errorObject "httpStatus")) {
                [void]$problems.Add("error.httpStatus must be absent for a local (no-request) error")
            }
            if ($ExpectedCause -ne "") {
                if (-not (Test-JsonProperty $errorObject "cause")) {
                    [void]$problems.Add("error.cause is missing (want '$ExpectedCause')")
                } else {
                    $actualCause = "$($errorObject.cause)"
                    if ($actualCause -ne $ExpectedCause) {
                        [void]$problems.Add("error.cause='$actualCause' want='$ExpectedCause'")
                    }
                }
            }
            if ($ExpectNoCause -and (Test-JsonProperty $errorObject "cause")) {
                [void]$problems.Add("error.cause must be absent for a local (no-request) error")
            }
            $leak = Test-SensitiveLeakage -Text $stderrText -Values $script:SensitiveValues
            if (-not [string]::IsNullOrEmpty($leak)) {
                [void]$problems.Add($leak)
            }
        }
    }

    # The query token must never appear on either protocol channel, regardless
    # of success or failure.
    $tokenLeak = Test-SensitiveLeakage -Text ($stdoutText + "`n" + $stderrText) -Values @($script:Token)
    if (-not [string]::IsNullOrEmpty($tokenLeak)) {
        [void]$problems.Add("query token leaked into process output")
    }

    if ($null -ne $Check) {
        try {
            $custom = & $Check $data $errorObject $Result
            if (-not [string]::IsNullOrEmpty($custom)) {
                [void]$problems.Add("$custom")
            }
        } catch {
            [void]$problems.Add("custom check threw: $($_.Exception.Message)")
        }
    }

    $resultText = "PASS"
    if ($problems.Count -gt 0) {
        $resultText = "FAIL"
        $script:Failures += "$Name :: $($problems -join '; ')"
    }
    $wantHttp = ""
    if ($ExpectedHttpStatus -ge 0) { $wantHttp = [string]$ExpectedHttpStatus }
    $record = [pscustomobject]@{
        Case       = $Name
        Command    = $Command
        ExitCode   = $Result.ExitCode
        WantExit   = $ExpectedExit
        Status     = $actualStatus
        WantStatus = $ExpectedStatus
        ErrorCode  = $actualCode
        WantCode   = $ExpectedCode
        HttpStatus = $actualHttpStatus
        WantHttp   = $wantHttp
        Cause      = $actualCause
        WantCause  = $ExpectedCause
        Result     = $resultText
        Reason     = ($problems -join "; ")
    }
    $script:Cases += $record
    if (-not $SkipSave) {
        Save-Result -Name $Name -Result $Result
    }
    return $record
}

# ---------------------------------------------------------------------------
# Offline self-test of the assertion engine (-SelfTest)
# ---------------------------------------------------------------------------
function Assert-SelfTest {
    param([bool]$Condition, [string]$Label)
    if ($Condition) {
        Write-Host "SELF-TEST PASS: $Label"
    } else {
        Write-Host "SELF-TEST FAIL: $Label"
        $script:SelfTestFailures++
    }
}

function New-TestResult {
    param([int]$ExitCode, [string]$Stdout, [string]$Stderr)
    return [pscustomobject]@{
        ExitCode = $ExitCode
        Stdout   = $Stdout
        Stderr   = $Stderr
        TimedOut = $false
    }
}

function Invoke-SelfTest {
    $savedCases = $script:Cases
    $savedFailures = $script:Failures
    $savedSensitive = $script:SensitiveValues
    $savedToken = $script:Token
    $script:Cases = @()
    $script:Failures = @()
    $script:SensitiveValues = @("imsi-999700000000001")
    $script:Token = ""
    try {
        # 1. Strict single-JSON-document parsing.
        $document = Get-JsonDocument -Text '{"data":{"status":"AVAILABLE"}}'
        Assert-SelfTest ($document.Valid -and "$($document.Value.data.status)" -eq "AVAILABLE") "single JSON document parses"
        $document = Get-JsonDocument -Text "  `n{`"a`":1}`n  "
        Assert-SelfTest ($document.Valid) "surrounding whitespace is allowed"
        $document = Get-JsonDocument -Text '{"a":"} {"}'
        Assert-SelfTest ($document.Valid) "braces inside a JSON string are handled"
        $document = Get-JsonDocument -Text '{"a":1} {"b":2}'
        Assert-SelfTest (-not $document.Valid -and $document.Reason -match "more than one JSON document") "multiple JSON documents rejected"
        $document = Get-JsonDocument -Text '{"a":1} trailing'
        Assert-SelfTest (-not $document.Valid) "trailing data rejected"
        $document = Get-JsonDocument -Text '{"a":1'
        Assert-SelfTest (-not $document.Valid -and $document.Reason -match "unterminated") "unterminated JSON rejected"
        $document = Get-JsonDocument -Text ''
        Assert-SelfTest (-not $document.Valid) "empty stream rejected"
        $document = Get-JsonDocument -Text '[1,2]'
        Assert-SelfTest (-not $document.Valid) "non-object JSON rejected for the envelope"

        # 2. Sensitive-leakage detector.
        Assert-SelfTest (-not [string]::IsNullOrEmpty((Test-SensitiveLeakage -Text 'error mentions imsi-999700000000001' -Values $script:SensitiveValues))) "subscriber leakage detected"
        Assert-SelfTest ($null -eq (Test-SensitiveLeakage -Text '{"error":{"code":"X"}}' -Values $script:SensitiveValues)) "clean error text accepted"

        # 3. Envelope/field assertions.
        $success = New-TestResult -ExitCode 0 -Stdout '{"data":{"type":"session","status":"AVAILABLE"}}' -Stderr ''
        $record = Add-Case -Name "selftest-success" -Command "synthetic" -Result $success -ExpectedExit 0 -ExpectedStatus "AVAILABLE" -ExpectedType "session" -SkipSave
        Assert-SelfTest ($record.Result -eq "PASS") "well-formed success envelope passes"

        $record = Add-Case -Name "selftest-success-bad-status" -Command "synthetic" -Result $success -ExpectedExit 0 -ExpectedStatus "UNAVAILABLE" -ExpectedType "session" -SkipSave
        Assert-SelfTest ($record.Result -eq "FAIL") "wrong data.status fails"

        $record = Add-Case -Name "selftest-success-stderr" -Command "synthetic" -Result (New-TestResult -ExitCode 0 -Stdout '{"data":{}}' -Stderr 'noise') -ExpectedExit 0 -SkipSave
        Assert-SelfTest ($record.Result -eq "FAIL") "non-empty stderr on success fails"

        $record = Add-Case -Name "selftest-success-two-docs" -Command "synthetic" -Result (New-TestResult -ExitCode 0 -Stdout '{"data":{}} {"data":{}}' -Stderr '') -ExpectedExit 0 -SkipSave
        Assert-SelfTest ($record.Result -eq "FAIL") "success with two JSON documents fails"

        $error = New-TestResult -ExitCode 4 -Stdout '' -Stderr '{"error":{"code":"OPENAMSM_HTTP_ERROR","httpStatus":404,"cause":"SESSION_NOT_FOUND"}}'
        $record = Add-Case -Name "selftest-error" -Command "synthetic" -Result $error -ExpectedExit 4 -ExpectedCode "OPENAMSM_HTTP_ERROR" -ExpectedHttpStatus 404 -ExpectedCause "SESSION_NOT_FOUND" -SkipSave
        Assert-SelfTest ($record.Result -eq "PASS") "well-formed error envelope passes"

        $record = Add-Case -Name "selftest-error-wrong-status" -Command "synthetic" -Result $error -ExpectedExit 4 -ExpectedCode "OPENAMSM_HTTP_ERROR" -ExpectedHttpStatus 409 -ExpectedCause "KEY_CONFLICT" -SkipSave
        Assert-SelfTest ($record.Result -eq "FAIL") "wrong error.httpStatus/cause fails"

        $record = Add-Case -Name "selftest-error-stdout" -Command "synthetic" -Result (New-TestResult -ExitCode 4 -Stdout '{"data":{}}' -Stderr '{"error":{"code":"X"}}') -ExpectedExit 4 -ExpectedCode "X" -SkipSave
        Assert-SelfTest ($record.Result -eq "FAIL") "non-empty stdout on failure fails"

        $record = Add-Case -Name "selftest-local-error" -Command "synthetic" -Result (New-TestResult -ExitCode 3 -Stdout '' -Stderr '{"error":{"code":"CONFIGURATION_MISSING"}}') -ExpectedExit 3 -ExpectedCode "CONFIGURATION_MISSING" -ExpectNoHttpStatus -ExpectNoCause -SkipSave
        Assert-SelfTest ($record.Result -eq "PASS") "local error without httpStatus/cause passes"

        $record = Add-Case -Name "selftest-local-error-transport" -Command "synthetic" -Result (New-TestResult -ExitCode 3 -Stdout '' -Stderr '{"error":{"code":"CONFIGURATION_MISSING","httpStatus":503}}') -ExpectedExit 3 -ExpectedCode "CONFIGURATION_MISSING" -ExpectNoHttpStatus -SkipSave
        Assert-SelfTest ($record.Result -eq "FAIL") "local error carrying httpStatus fails"

        $record = Add-Case -Name "selftest-leak" -Command "synthetic" -Result (New-TestResult -ExitCode 4 -Stdout '' -Stderr '{"error":{"code":"OPENAMSM_HTTP_ERROR","cause":"imsi-999700000000001"}}') -ExpectedExit 4 -ExpectedCode "OPENAMSM_HTTP_ERROR" -SkipSave
        Assert-SelfTest ($record.Result -eq "FAIL") "subscriber leakage into error text fails"

        $record = Add-Case -Name "selftest-not-run" -Command "synthetic" -Result $null -ExpectedExit 4 -NotRunReason "optional input missing" -SkipSave
        Assert-SelfTest ($record.Result -eq "NOT_RUN" -and $record.Reason -ne "") "optional scenario recorded as NOT_RUN with a reason"

        $script:Token = "agent-secret-token"
        $record = Add-Case -Name "selftest-token-leak" -Command "synthetic" -Result (New-TestResult -ExitCode 0 -Stdout '{"data":{"token":"agent-secret-token"}}' -Stderr '') -ExpectedExit 0 -SkipSave
        Assert-SelfTest ($record.Result -eq "FAIL") "query token leakage into process output fails"
        $script:Token = ""

        $record = Add-Case -Name "selftest-custom-check" -Command "synthetic" -Result $success -ExpectedExit 0 -Check {
            param($data, $errorObject, $rawResult)
            if ("$($data.status)" -ne "AVAILABLE") { return "custom status mismatch" }
            return $null
        } -SkipSave
        Assert-SelfTest ($record.Result -eq "PASS") "custom check can pass"
    } finally {
        $script:Cases = $savedCases
        $script:Failures = $savedFailures
        $script:SensitiveValues = $savedSensitive
        $script:Token = $savedToken
    }
}

# ---------------------------------------------------------------------------
# -SelfTest: verify the assertion engine offline, then exit.
# ---------------------------------------------------------------------------
if ($SelfTest) {
    Invoke-SelfTest
    if ($script:SelfTestFailures -eq 0) {
        Write-Host "SELF-TEST RESULT: PASS"
        exit 0
    }
    Write-Host "SELF-TEST RESULT: FAIL ($($script:SelfTestFailures) failed assertion(s))"
    exit 1
}

# ---------------------------------------------------------------------------
# Compute the effective test token and sensitive-value list (never printed).
# ---------------------------------------------------------------------------
$script:Token = [Environment]::GetEnvironmentVariable("CA_OPENAMSM_QUERY_TOKEN")
if ([string]::IsNullOrEmpty($script:Token)) { $script:Token = "" }
$imsiForLeak = $Supi -replace '^imsi-', ''
$sensitive = @($script:Token, $Supi, $imsiForLeak, $Ipv4, $WrongIpv4, $UnallocatedIpv6, $WrongSupi, "definitely-wrong-token")
$script:SensitiveValues = @()
foreach ($value in $sensitive) {
    if (-not [string]::IsNullOrEmpty($value)) { $script:SensitiveValues += $value }
}

# ---------------------------------------------------------------------------
# Required runner inputs
# ---------------------------------------------------------------------------
$missingInputs = @()
foreach ($pair in @(
    @{ Name = "-Exe"; Value = $Exe },
    @{ Name = "-AmfMetricsOrigin"; Value = $AmfMetricsOrigin },
    @{ Name = "-SmfMetricsOrigin"; Value = $SmfMetricsOrigin },
    @{ Name = "-SmfSbiOrigin"; Value = $SmfSbiOrigin },
    @{ Name = "-Supi"; Value = $Supi }
)) {
    if ([string]::IsNullOrWhiteSpace([string]$pair.Value)) { $missingInputs += $pair.Name }
}
if ($missingInputs.Count -gt 0) {
    Fail-Runner -Message "Missing required acceptance parameter(s): $($missingInputs -join ', ')" -ExitCode 2
}
if ($Supi -notmatch '^imsi-[0-9]{1,15}$') {
    Fail-Runner -Message "-Supi must use the OpenAMSM imsi- form with 1-15 decimal digits" -ExitCode 2
}
if ($Psi -lt 0 -or $Psi -gt 254) {
    Fail-Runner -Message "-Psi must be in [0, 254] so the wrong-PSI probe (Psi+1) stays in the source range" -ExitCode 2
}
if ($WrongSupi -eq $Supi) {
    Fail-Runner -Message "-WrongSupi must differ from -Supi" -ExitCode 2
}
if ([string]::IsNullOrWhiteSpace($ExpectedGnbName)) {
    Fail-Runner -Message "-ExpectedGnbName must not be empty" -ExitCode 2
}
if ([string]::IsNullOrWhiteSpace($ExpectedPduState)) {
    Fail-Runner -Message "-ExpectedPduState must not be empty" -ExitCode 2
}
$script:ExePath = (Resolve-Path -LiteralPath $Exe).Path
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
$OutDir = (Resolve-Path -LiteralPath $OutDir).Path

if ([string]::IsNullOrEmpty($script:Token)) {
    Fail-Runner -Message "CA_OPENAMSM_QUERY_TOKEN must be injected into this process environment (it is never taken from a file/argument)." -ExitCode 3
}

# The local "no request sent" proof uses an intentionally unreachable closed
# loopback origin. If a request were attempted the CLI would fail with a
# transport error (exit 4, OPENAMSM_TRANSPORT_ERROR) instead of the asserted
# local configuration/argument error.
$script:ClosedLoopbackOrigin = "http://127.0.0.1:1"

$common = @{
    CA_OPENAMSM_AMF_METRICS_ORIGIN = $AmfMetricsOrigin
    CA_OPENAMSM_SMF_METRICS_ORIGIN = $SmfMetricsOrigin
    CA_OPENAMSM_SMF_SBI_ORIGIN     = $SmfSbiOrigin
}
$withToken = $common.Clone()
$withToken["CA_OPENAMSM_QUERY_TOKEN"] = $script:Token
$closedOrigins = @{
    CA_OPENAMSM_AMF_METRICS_ORIGIN = $script:ClosedLoopbackOrigin
    CA_OPENAMSM_SMF_METRICS_ORIGIN = $script:ClosedLoopbackOrigin
    CA_OPENAMSM_SMF_SBI_ORIGIN     = $script:ClosedLoopbackOrigin
}
$closedWithToken = $closedOrigins.Clone()
$closedWithToken["CA_OPENAMSM_QUERY_TOKEN"] = $script:Token

# 1. CLI process contract (human-readable output, exit 0)
$version = Invoke-Cli -Arguments @("--version") -Environment @{}
Add-Case -Name "version" -Command "ca-client-tools --version" -Result $version -ExpectedExit 0 -HumanOutput -Check {
    param($data, $errorObject, $rawResult)
    if (-not $rawResult.Stdout.Trim().StartsWith("ca-client-tools")) {
        return "version output does not start with 'ca-client-tools'"
    }
    return $null
} | Out-Null
$help = Invoke-Cli -Arguments @("--help") -Environment @{}
Add-Case -Name "help" -Command "ca-client-tools --help" -Result $help -ExpectedExit 0 -HumanOutput -Check {
    param($data, $errorObject, $rawResult)
    if (-not $rawResult.Stdout.Contains("Usage:")) {
        return "help output does not contain 'Usage:'"
    }
    return $null
} | Out-Null

# 2. Metrics list surfaces (plain HTTP/1.1 clients, separate from SBI transport)
$ue = Invoke-Cli -Arguments @("amf", "ue", "list", "--page", "0", "--page-size", "100") -Environment $common
Add-Case -Name "amf-ue-list" -Command "amf ue list" -Result $ue -ExpectedExit 0 -Check {
    param($data, $errorObject, $rawResult)
    if ($null -eq $data) { return "data envelope missing" }
    if (-not (Test-JsonProperty $data "items")) { return "data.items missing" }
    if (-not (Test-JsonProperty $data "pager")) { return "data.pager missing" }
    $item = Get-ItemByField -Items $data.items -Field "supi" -Value $Supi
    if ($null -eq $item) { return "no AMF UE item with data.items[].supi == $Supi" }
    if (-not (Test-JsonProperty $item "suci") -or [string]::IsNullOrEmpty("$($item.suci)")) {
        return "data.items[].suci missing or empty for supi $Supi"
    }
    if (-not (Test-JsonProperty $item "pdu_sessions_count")) { return "data.items[].pdu_sessions_count missing" }
    $sessionCount = $item.pdu_sessions_count
    if ($null -eq $sessionCount -or -not ($sessionCount -is [int] -or $sessionCount -is [long] -or $sessionCount -is [double]) -or [int]$sessionCount -lt 1) {
        return "data.items[].pdu_sessions_count must be numeric and >= 1, got '$sessionCount'"
    }
    $psis = @()
    foreach ($session in @($item.pdu_sessions)) {
        if ($null -ne $session -and (Test-JsonProperty $session "psi")) {
            $psis += [int]$session.psi
        }
    }
    if ($psis -notcontains $Psi) {
        return "data.items[].pdu_sessions[].psi does not contain $Psi (got '$($psis -join ',')')"
    }
    return $null
} | Out-Null

$gnb = Invoke-Cli -Arguments @("amf", "gnb", "list", "--page", "0", "--page-size", "100") -Environment $common
Add-Case -Name "amf-gnb-list" -Command "amf gnb list" -Result $gnb -ExpectedExit 0 -Check {
    param($data, $errorObject, $rawResult)
    if ($null -eq $data) { return "data envelope missing" }
    if (-not (Test-JsonProperty $data "items")) { return "data.items missing" }
    if (-not (Test-JsonProperty $data "pager")) { return "data.pager missing" }
    $item = Get-ItemByField -Items $data.items -Field "name" -Value $ExpectedGnbName
    if ($null -eq $item) {
        $names = @()
        foreach ($candidate in @($data.items)) {
            if ($null -ne $candidate) { $names += "$($candidate.name)" }
        }
        return "no AMF gNB item with data.items[].name == '$ExpectedGnbName' (got '$($names -join ',')')"
    }
    if (-not (Test-JsonProperty $item "gnb_id")) { return "data.items[].gnb_id missing" }
    if (-not (Test-JsonProperty $item "num_connected_ues")) { return "data.items[].num_connected_ues missing" }
    $connected = $item.num_connected_ues
    if ($null -eq $connected -or -not ($connected -is [int] -or $connected -is [long] -or $connected -is [double]) -or [int]$connected -lt 1) {
        return "data.items[].num_connected_ues must be numeric and >= 1, got '$connected'"
    }
    return $null
} | Out-Null

$pdu = Invoke-Cli -Arguments @("smf", "pdu", "list", "--page", "0", "--page-size", "100") -Environment $common
Add-Case -Name "smf-pdu-list" -Command "smf pdu list" -Result $pdu -ExpectedExit 0 -Check {
    param($data, $errorObject, $rawResult)
    if ($null -eq $data) { return "data envelope missing" }
    if (-not (Test-JsonProperty $data "items")) { return "data.items missing" }
    if (-not (Test-JsonProperty $data "pager")) { return "data.pager missing" }
    $match = $null
    foreach ($item in @($data.items)) {
        if ($null -eq $item -or "$($item.supi)" -ne $Supi) { continue }
        foreach ($entry in @($item.pdu)) {
            if ($null -eq $entry) { continue }
            if ((Test-JsonProperty $entry "psi") -and [int]$entry.psi -eq $Psi) {
                $match = $entry
                break
            }
        }
        if ($null -ne $match) { break }
    }
    if ($null -eq $match) { return "no SMF PDU item for supi=$Supi with pdu[].psi=$Psi" }
    if (-not (Test-JsonProperty $match "pdu_state")) { return "data.items[].pdu[].pdu_state missing" }
    if ("$($match.pdu_state)" -ne $ExpectedPduState) {
        return "data.items[].pdu[].pdu_state='$($match.pdu_state)' want='$ExpectedPduState'"
    }
    if ($Ipv4 -ne "") {
        if (-not (Test-JsonProperty $match "ipv4")) { return "data.items[].pdu[].ipv4 missing" }
        if ("$($match.ipv4)" -ne $Ipv4) {
            return "data.items[].pdu[].ipv4='$($match.ipv4)' want='$Ipv4'"
        }
    }
    return $null
} | Out-Null

# 3. Eight SMF context queries with the real SUPI + PSI
$types = @("session", "policy", "up-node", "signaling-session", "ip-address", "up-selection", "charging", "session-auth")
$identityBody = (@{ supi = $Supi; pduSessionId = $Psi } | ConvertTo-Json -Compress)
foreach ($type in $types) {
    $result = Invoke-Cli -Arguments @("smf", "context", "query", "--type", $type) -Environment $withToken -StdinText $identityBody
    Add-Case -Name "context-$type" -Command "smf context query --type $type" -Result $result -ExpectedExit 0 -ExpectedStatus "AVAILABLE" -ExpectedType $type -Check {
        param($data, $errorObject, $rawResult)
        if ($null -eq $data) { return "data envelope missing" }
        if (-not (Test-JsonProperty $data "key")) { return "data.key missing" }
        if (-not (Test-JsonProperty $data "context")) { return "data.context missing" }
        if ("$($data.key.supi)" -ne $Supi) {
            return "data.key.supi='$($data.key.supi)' want='$Supi'"
        }
        if (-not (Test-JsonProperty $data.key "pduSessionId")) { return "data.key.pduSessionId missing" }
        if ([int]$data.key.pduSessionId -ne $Psi) {
            return "data.key.pduSessionId=$($data.key.pduSessionId) want=$Psi"
        }
        return $null
    } | Out-Null
}

# 4. Alternative real lookup shapes
$imsi = $Supi -replace '^imsi-', ''
$normalizedSupi = "imsi-$imsi"
$imsiBody = (@{ imsi = $imsi; pduSessionId = $Psi } | ConvertTo-Json -Compress)
$r = Invoke-Cli -Arguments @("smf", "context", "query", "--type", "session") -Environment $withToken -StdinText $imsiBody
Add-Case -Name "context-imsi-psi" -Command "imsi + PSI" -Result $r -ExpectedExit 0 -ExpectedStatus "AVAILABLE" -ExpectedType "session" -Check {
    param($data, $errorObject, $rawResult)
    if ($null -eq $data) { return "data envelope missing" }
    if (-not (Test-JsonProperty $data "key")) { return "data.key missing" }
    if ("$($data.key.supi)" -ne $normalizedSupi) {
        return "data.key.supi='$($data.key.supi)' want normalized SUPI='$normalizedSupi'"
    }
    if (-not (Test-JsonProperty $data.key "pduSessionId")) { return "data.key.pduSessionId missing" }
    if ([int]$data.key.pduSessionId -ne $Psi) {
        return "data.key.pduSessionId=$($data.key.pduSessionId) want=$Psi"
    }
    return $null
} | Out-Null

if ($Ipv4 -ne "") {
    $ipv4Body = (@{ supi = $Supi; pduSessionId = $Psi; ueIpv4 = $Ipv4 } | ConvertTo-Json -Compress)
    $r = Invoke-Cli -Arguments @("smf", "context", "query", "--type", "session") -Environment $withToken -StdinText $ipv4Body
    Add-Case -Name "context-supi-psi-ipv4" -Command "SUPI + PSI + IPv4" -Result $r -ExpectedExit 0 -ExpectedStatus "AVAILABLE" -ExpectedType "session" -Check {
        param($data, $errorObject, $rawResult)
        if ($null -eq $data) { return "data envelope missing" }
        if (-not (Test-JsonProperty $data "key")) { return "data.key missing" }
        if ("$($data.key.supi)" -ne $Supi) { return "data.key.supi='$($data.key.supi)' want='$Supi'" }
        if ([int]$data.key.pduSessionId -ne $Psi) { return "data.key.pduSessionId=$($data.key.pduSessionId) want=$Psi" }
        if ("$($data.key.ueIpv4)" -ne $Ipv4) { return "data.key.ueIpv4='$($data.key.ueIpv4)' want='$Ipv4'" }
        return $null
    } | Out-Null

    $ipv4OnlyBody = (@{ ueIpv4 = $Ipv4 } | ConvertTo-Json -Compress)
    $r = Invoke-Cli -Arguments @("smf", "context", "query", "--type", "session") -Environment $withToken -StdinText $ipv4OnlyBody
    Add-Case -Name "context-ipv4-only" -Command "IPv4 only" -Result $r -ExpectedExit 0 -ExpectedStatus "AVAILABLE" -ExpectedType "session" -Check {
        param($data, $errorObject, $rawResult)
        if ($null -eq $data) { return "data envelope missing" }
        if (-not (Test-JsonProperty $data "key")) { return "data.key missing" }
        if ("$($data.key.ueIpv4)" -ne $Ipv4) { return "data.key.ueIpv4='$($data.key.ueIpv4)' want='$Ipv4'" }
        return $null
    } | Out-Null
} else {
    Add-Case -Name "context-supi-psi-ipv4" -Command "SUPI + PSI + IPv4" -Result $null -ExpectedExit 0 -NotRunReason "-Ipv4 was not provided; IPv4 identity echo not asserted" | Out-Null
    Add-Case -Name "context-ipv4-only" -Command "IPv4 only" -Result $null -ExpectedExit 0 -NotRunReason "-Ipv4 was not provided; IPv4-only lookup not asserted" | Out-Null
}

# 5. Negative lookups: real source ProblemDetails preserved as remote errors
$wrongPsiBody = (@{ supi = $Supi; pduSessionId = ($Psi + 1) } | ConvertTo-Json -Compress)
$r = Invoke-Cli -Arguments @("smf", "context", "query", "--type", "session") -Environment $withToken -StdinText $wrongPsiBody
Add-Case -Name "negative-wrong-psi" -Command "wrong PSI" -Result $r -ExpectedExit 4 -ExpectedCode "OPENAMSM_HTTP_ERROR" -ExpectedHttpStatus 404 -ExpectedCause "SESSION_NOT_FOUND" | Out-Null

$wrongSupiBody = (@{ supi = $WrongSupi; pduSessionId = $Psi } | ConvertTo-Json -Compress)
$r = Invoke-Cli -Arguments @("smf", "context", "query", "--type", "session") -Environment $withToken -StdinText $wrongSupiBody
Add-Case -Name "negative-wrong-supi" -Command "wrong SUPI" -Result $r -ExpectedExit 4 -ExpectedCode "OPENAMSM_HTTP_ERROR" -ExpectedHttpStatus 404 -ExpectedCause "SESSION_NOT_FOUND" | Out-Null

if ($WrongIpv4 -ne "") {
    $unallocatedIpv4Body = (@{ supi = $Supi; pduSessionId = $Psi; ueIpv4 = $WrongIpv4 } | ConvertTo-Json -Compress)
    $r = Invoke-Cli -Arguments @("smf", "context", "query", "--type", "session") -Environment $withToken -StdinText $unallocatedIpv4Body
    Add-Case -Name "negative-unallocated-ipv4" -Command "unallocated IPv4" -Result $r -ExpectedExit 4 -ExpectedCode "OPENAMSM_HTTP_ERROR" -ExpectedHttpStatus 409 -ExpectedCause "KEY_CONFLICT" | Out-Null
} else {
    Add-Case -Name "negative-unallocated-ipv4" -Command "unallocated IPv4" -Result $null -ExpectedExit 4 -ExpectedCode "OPENAMSM_HTTP_ERROR" -NotRunReason "-WrongIpv4 was not provided; unallocated-IPv4 negative not exercised" | Out-Null
}

if ($UnallocatedIpv6 -ne "") {
    $unallocatedIpv6Body = (@{ supi = $Supi; pduSessionId = $Psi; ueIpv6 = $UnallocatedIpv6 } | ConvertTo-Json -Compress)
    $r = Invoke-Cli -Arguments @("smf", "context", "query", "--type", "session") -Environment $withToken -StdinText $unallocatedIpv6Body
    Add-Case -Name "negative-unallocated-ipv6" -Command "unallocated IPv6" -Result $r -ExpectedExit 4 -ExpectedCode "OPENAMSM_HTTP_ERROR" -ExpectedHttpStatus 409 -ExpectedCause "KEY_CONFLICT" | Out-Null
} else {
    Add-Case -Name "negative-unallocated-ipv6" -Command "unallocated IPv6" -Result $null -ExpectedExit 4 -ExpectedCode "OPENAMSM_HTTP_ERROR" -NotRunReason "-UnallocatedIpv6 was not provided; unallocated-IPv6 negative not exercised" | Out-Null
}

# 6. Token rejection: wrong token reaches the real source as 403
$wrongToken = $common.Clone()
$wrongToken["CA_OPENAMSM_QUERY_TOKEN"] = "definitely-wrong-token"
$r = Invoke-Cli -Arguments @("smf", "context", "query", "--type", "session") -Environment $wrongToken -StdinText $identityBody
Add-Case -Name "negative-wrong-token" -Command "wrong token" -Result $r -ExpectedExit 4 -ExpectedCode "OPENAMSM_HTTP_ERROR" -ExpectedHttpStatus 403 -ExpectedCause "FORBIDDEN" | Out-Null

# 7. Local rejection before any network access. The origins point at a closed
#    loopback port, so a request attempt would produce a transport error; the
#    asserted local error code/exit and absent httpStatus/cause prove that no
#    request was sent.
$r = Invoke-Cli -Arguments @("smf", "context", "query", "--type", "session") -Environment $closedOrigins -RemoveEnvironment @("CA_OPENAMSM_QUERY_TOKEN") -StdinText $identityBody
Add-Case -Name "negative-missing-token" -Command "missing token" -Result $r -ExpectedExit 3 -ExpectedCode "CONFIGURATION_MISSING" -ExpectNoHttpStatus -ExpectNoCause | Out-Null

$r = Invoke-Cli -Arguments @("smf", "context", "query", "--type", "not-a-context") -Environment $closedWithToken -StdinText $identityBody
Add-Case -Name "invalid-context-type" -Command "unknown --type" -Result $r -ExpectedExit 2 -ExpectedCode "INVALID_ARGUMENT" -ExpectNoHttpStatus -ExpectNoCause | Out-Null

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
$script:Cases | Format-Table -AutoSize | Out-String | Write-Host
$passCount = @($script:Cases | Where-Object { $_.Result -eq "PASS" }).Count
$failCount = @($script:Cases | Where-Object { $_.Result -eq "FAIL" }).Count
$notRunCount = @($script:Cases | Where-Object { $_.Result -eq "NOT_RUN" }).Count
$overall = "PASS"
if ($failCount -gt 0) {
    $overall = "FAIL"
} elseif ($notRunCount -gt 0) {
    $overall = "PARTIAL"
}
Write-Host ""
Write-Host ("RESULT: {0} -- PASS={1} FAIL={2} NOT_RUN={3} TOTAL={4}" -f $overall, $passCount, $failCount, $notRunCount, $script:Cases.Count)
foreach ($notRun in @($script:Cases | Where-Object { $_.Result -eq "NOT_RUN" })) {
    Write-Host ("NOT RUN: {0} ({1})" -f $notRun.Case, $notRun.Reason)
}
if ($script:Failures.Count -gt 0) {
    Write-Host "FAILURES:"
    foreach ($failure in $script:Failures) { Write-Host "  - $failure" }
}

$summary = [pscustomobject]@{
    Exe              = $script:ExePath
    Supi             = $Supi
    Psi              = $Psi
    Ipv4             = $Ipv4
    WrongIpv4        = $WrongIpv4
    UnallocatedIpv6  = $UnallocatedIpv6
    WrongSupi        = $WrongSupi
    ExpectedGnbName  = $ExpectedGnbName
    ExpectedPduState = $ExpectedPduState
    Counts           = [pscustomobject]@{
        Total   = $script:Cases.Count
        PASS    = $passCount
        FAIL    = $failCount
        NOT_RUN = $notRunCount
    }
    Cases            = $script:Cases
    Failures         = $script:Failures
    Result           = $overall
    GeneratedAtLocal = (Get-Date).ToString("o")
}
$summary | ConvertTo-Json -Depth 6 | Set-Content -Path (Join-Path $OutDir "summary.json") -Encoding utf8

if ($failCount -gt 0) {
    exit 1
}
exit 0

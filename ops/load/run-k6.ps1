param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('read', 'mixed', 'public')]
    [string]$Scenario,
    [ValidateRange(1, 100)]
    [int]$VUs = 1,
    [string]$Duration = '10s',
    [ValidateRange(0, 60)]
    [double]$PauseSeconds = 0.2,
    [string]$RunId
)

$ErrorActionPreference = 'Stop'
if ($Duration -notmatch '^([1-9][0-9]*)(s|m)$') { throw 'Duration must be like 10s or 5m.' }
$seconds = [int]$Matches[1] * $(if ($Matches[2] -eq 'm') { 60 } else { 1 })
if ($seconds -gt 840) { throw 'Runs must be at most 14 minutes; access JWT expires after 15 minutes.' }

$root = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$envFile = Join-Path $root '.env.load'
if (-not (Test-Path -LiteralPath $envFile)) { throw 'Create .env.load first.' }
$runId = if ($RunId) { $RunId } else { '{0}-{1}-{2}' -f $Scenario, (Get-Date -Format 'yyyyMMdd-HHmmss'), ([guid]::NewGuid().ToString('N').Substring(0, 8)) }
if ($runId -cnotmatch ('^' + $Scenario + '-[0-9]{8}-[0-9]{6}-[0-9a-f]{8}$')) { throw 'RunId must match scenario-YYYYMMDD-HHMMSS-8hex.' }
$resultDir = Join-Path $root "load-results/$runId"
if (Test-Path -LiteralPath $resultDir) { throw "Result directory already exists: $resultDir" }
New-Item -ItemType Directory -Path $resultDir | Out-Null
$env:K6_RUN_ID = $runId
$env:K6_PAUSE_SECONDS = [string]$PauseSeconds
$compose = @('compose', '--project-name', 'ai-buddy-load', '--file', (Join-Path $root 'compose.load.yaml'), '--env-file', $envFile, '--profile', 'load-tools')
Write-Host "Scenario=$Scenario VUs=$VUs Duration=$Duration Results=$resultDir"
$savedErrorAction = $ErrorActionPreference
try {
    # Windows PowerShell treats some native stderr lines as ErrorRecords.
    # Keep the Docker exit code authoritative and retain its full console log.
    $ErrorActionPreference = 'Continue'
    & docker @compose run --rm k6 run --vus $VUs --duration $Duration --summary-export "/results/$runId/summary.json" --out "json=/results/$runId/metrics.ndjson" "/scripts/$Scenario.js" 2>&1 |
        Tee-Object -LiteralPath (Join-Path $resultDir 'k6-console.txt')
    $k6ExitCode = $LASTEXITCODE
} finally {
    $ErrorActionPreference = $savedErrorAction
}
if ($k6ExitCode -ne 0) { throw "k6 failed with exit code $k6ExitCode; inspect $resultDir/k6-console.txt and available reports" }
Write-Host "Report saved: $resultDir"

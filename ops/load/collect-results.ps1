param(
    [Parameter(Mandatory = $true)][string]$RunId,
    [Parameter(Mandatory = $true)][datetimeoffset]$StartedAt,
    [Parameter(Mandatory = $true)][datetimeoffset]$EndedAt,
    [string]$PrometheusURL = 'http://127.0.0.1:19090'
)

$ErrorActionPreference = 'Stop'
$root = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$dir = Join-Path $root "load-results/$RunId"
$summaryPath = Join-Path $dir 'summary.json'
if (-not (Test-Path -LiteralPath $summaryPath)) { throw "Missing k6 summary: $summaryPath" }
if ((Test-Path -LiteralPath (Join-Path $dir 'result.json')) -or (Test-Path -LiteralPath (Join-Path $dir 'server-series.json'))) { throw "Collected results already exist for $RunId" }
$summary = Get-Content -Raw -Encoding utf8 -LiteralPath $summaryPath | ConvertFrom-Json
$client = [ordered]@{
    source = 'k6 phase=workload'
    requests = $summary.metrics.workload_requests.count
    rps = $summary.metrics.workload_requests.rate
    latency_p50_ms = $summary.metrics.'http_req_duration{phase:workload}'.med
    latency_p95_ms = $summary.metrics.'http_req_duration{phase:workload}'.'p(95)'
    latency_p99_ms = $summary.metrics.'http_req_duration{phase:workload}'.'p(99)'
    http_error_rate = $summary.metrics.'http_req_failed{phase:workload}'.value
    functional_success_rate = $summary.metrics.functional_success.value
}
if ($null -eq $client.requests -or $null -eq $client.latency_p99_ms -or $null -eq $client.http_error_rate -or $null -eq $client.functional_success_rate) { throw 'k6 summary is missing phase-filtered metrics.' }

# A 15 s rate window needs at least two 5 s scrapes. Start server analysis
# after its first window so samples cannot include the previous run.
$serverStart = $StartedAt.AddSeconds(15)
if ($serverStart -ge $EndedAt) { throw 'Run is too short for server rate series; use a duration over 15 seconds.' }
$start = $serverStart.ToUnixTimeSeconds()
$end = $EndedAt.ToUnixTimeSeconds()
$filter = 'job="ai-buddy-load-backend",route!="GET /health",route!="POST /api/auth/login"'
$requests = "rate(http_requests_total{$filter}[15s])"
$bucket = "rate(http_request_duration_seconds_bucket{$filter}[15s])"
$queries = [ordered]@{
    backend_rps = "sum($requests) or vector(0)"
    backend_latency_p50_seconds = "histogram_quantile(0.50, sum by (le) ($bucket))"
    backend_latency_p95_seconds = "histogram_quantile(0.95, sum by (le) ($bucket))"
    backend_latency_p99_seconds = "histogram_quantile(0.99, sum by (le) ($bucket))"
    backend_http_error_rate = "(sum(rate(http_requests_total{$filter,status=~`"[45]..`"}[15s])) or vector(0)) / clamp_min(sum($requests) or vector(0), 1e-9)"
    backend_login_rps = 'sum(rate(http_requests_total{job="ai-buddy-load-backend",route="POST /api/auth/login"}[15s])) or vector(0)'
    backend_cpu_cores = 'rate(process_cpu_seconds_total{job="ai-buddy-load-backend"}[15s])'
    backend_ram_mib = 'process_resident_memory_bytes{job="ai-buddy-load-backend"} / 1024 / 1024'
}
$series = [ordered]@{}
foreach ($key in $queries.Keys) {
    $query = $queries[$key]
    $uri = "$PrometheusURL/api/v1/query_range?query=$([uri]::EscapeDataString($query))&start=$start&end=$end&step=5"
    $response = Invoke-RestMethod -Uri $uri -TimeoutSec 30
    if ($response.status -ne 'success') { throw "Prometheus query failed: $key" }
    $hasSamples = (
        $response.data.result -and
        $response.data.result[0].values
    )

    $optionalPublicMetrics = @(
        'backend_latency_p50_seconds',
        'backend_latency_p95_seconds',
        'backend_latency_p99_seconds'
    )

    $isOptional = (
        $RunId -match '^public-' -and
        $key -in $optionalPublicMetrics
    )

    if (-not $hasSamples) {
        if (-not $isOptional) {
            throw "Prometheus returned no samples: $key"
        }

        Write-Warning "No API traffic for $key in public scenario"

        $series[$key] = [ordered]@{
            source = 'Prometheus backend'
            query = $query
            data = @()
            note = 'No matching user API requests'
        }

        continue
    }

    $series[$key] = [ordered]@{
        source = 'Prometheus backend'
        query = $query
        data = $response.data.result
    }

}
$series | ConvertTo-Json -Depth 100 | Set-Content -LiteralPath (Join-Path $dir 'server-series.json') -Encoding utf8
[ordered]@{
    run_id = $RunId
    started_at = $StartedAt.ToString('o')
    ended_at = $EndedAt.ToString('o')
    server_analysis_from = $serverStart.ToString('o')
    client = $client
    server_series_file = 'server-series.json'
    note = 'Client metrics are k6 phase=workload. Server series exclude health and login; first 15 s omitted to avoid previous-run rate bleed.'
} | ConvertTo-Json -Depth 20 | Set-Content -LiteralPath (Join-Path $dir 'result.json') -Encoding utf8
Write-Host "Collected client summary and $($series.Count) Prometheus series for $RunId"

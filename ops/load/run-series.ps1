param([switch]$Execute, [switch]$Resume, [switch]$AllowChangedConfig,
    [ValidateRange(1, 9)][int]$MaxRuns = 9)

if ($AllowChangedConfig -and -not $Resume) { throw '-AllowChangedConfig requires -Resume.' }

$ErrorActionPreference = 'Stop'
$root = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$envFile = Join-Path $root '.env.load'
if (-not (Test-Path -LiteralPath $envFile)) { throw 'Create .env.load first.' }
$envLines = Get-Content -LiteralPath $envFile
function EnvValue([string]$name, [string]$fallback) {
    $line = $envLines | Where-Object { $_ -match "^$name=" } | Select-Object -Last 1
    if ($line) { return ($line -split '=', 2)[1].Trim() }
    return $fallback
}
$backendPort = EnvValue 'LOAD_BACKEND_PORT' '18080'
$promPort = EnvValue 'LOAD_PROMETHEUS_PORT' '19090'
$grafanaPort = EnvValue 'LOAD_GRAFANA_PORT' '13000'
$promURL = "http://127.0.0.1:$promPort"
$compose = @('compose', '--project-name', 'ai-buddy-load', '--file', (Join-Path $root 'compose.load.yaml'), '--env-file', $envFile, '--profile', 'load-tools')
$scenarios = @('public', 'read', 'mixed')
$levels = @(10, 25, 50)
$duration = '3m'
$pauseSeconds = 30

function DatabaseSnapshot {
    $sql = "SELECT current_database(), (SELECT count(*) FROM generate_series(1,100) s WHERE EXISTS (SELECT 1 FROM users WHERE email='load.intern.' || lpad(s::text,3,'0') || '@example.invalid')), (SELECT count(*) FROM users WHERE email LIKE 'load.intern.%@example.invalid'), (SELECT count(*) FROM notes n JOIN users u ON u.id=n.user_id WHERE u.email LIKE 'load.intern.%@example.invalid'), (SELECT count(*) FROM calendar_events e JOIN users u ON u.id=e.user_id WHERE u.email LIKE 'load.intern.%@example.invalid')"
    $line = (& docker @compose exec -T postgres psql -U load_user -d ai_buddy_load -X -A -t -v ON_ERROR_STOP=1 -c $sql).Trim()
    if ($LASTEXITCODE -ne 0) { throw 'Database preflight failed.' }
    $fields = $line -split '\|'
    if ($fields.Count -ne 5 -or $fields[0] -ne 'ai_buddy_load' -or [int]$fields[1] -ne 100 -or [int]$fields[2] -ne 100) { throw "Unexpected database or user count: $line" }
    return [ordered]@{ database = $fields[0]; users = [int]$fields[1]; notes = [int]$fields[3]; events = [int]$fields[4] }
}

function Preflight {
    & docker @compose config --quiet
    if ($LASTEXITCODE -ne 0) { throw 'Compose config failed.' }
    $backend = Invoke-RestMethod -Uri "http://127.0.0.1:$backendPort/health" -TimeoutSec 10
    if ($backend.status -ne 'ok') { throw 'Backend health failed.' }
    $targets = (Invoke-RestMethod -Uri "$promURL/api/v1/targets?state=active" -TimeoutSec 10).data.activeTargets
    $target = $targets | Where-Object { $_.labels.job -eq 'ai-buddy-load-backend' -and $_.scrapeUrl -eq 'http://backend:8080/metrics' } | Select-Object -First 1
    if (-not $target -or $target.health -ne 'up') { throw 'Load backend Prometheus target is not UP.' }
    $grafana = Invoke-RestMethod -Uri "http://127.0.0.1:$grafanaPort/api/health" -TimeoutSec 10
    if ($grafana.database -ne 'ok') { throw 'Grafana health failed.' }
    return DatabaseSnapshot
}

function CleanupPreview([string]$runId) {
    $lines = & docker @compose run --rm cleanup-load --run-id $runId
    if ($LASTEXITCODE -ne 0) { throw "Cleanup dry-run failed for $runId" }
    $line = $lines | Where-Object { $_ -match '^DRY RUN database=ai_buddy_load ' } | Select-Object -Last 1
    if (-not $line) { throw 'Cleanup preview was empty.' }
    $parsed = [regex]::Match($line, 'notes=([0-9]+) events=([0-9]+); no rows deleted')
    if (-not $parsed.Success) { throw "Unexpected cleanup preview: $line" }
    return [ordered]@{ notes = [int64]$parsed.Groups[1].Value; events = [int64]$parsed.Groups[2].Value }
}

function ConfigFingerprint {
    $paths = @(
        'compose.load.yaml', 'ops/load/prometheus.yml',
        'ops/load/k6/common.js', 'ops/load/k6/public.js',
        'ops/load/k6/read.js', 'ops/load/k6/mixed.js',
        'ops/load/run-k6.ps1', 'ops/load/collect-results.ps1',
        'ops/load/run-series.ps1', 'ops/load/seed.Dockerfile',
        'apps/backend/cmd/cleanup-load/main.go', 'apps/backend/cmd/seed-load/main.go',
        'apps/backend/go.mod', 'apps/backend/go.sum',
        'apps/backend/internal/middleware/metrics.go',
        'apps/backend/internal/handler/router.go', 'apps/backend/internal/app/app.go',
        'apps/backend/internal/security/password.go', 'apps/backend/internal/service/auth.go',
        'ops/load/grafana/dashboards/backend-performance.json'
    )
    $hashes = [ordered]@{}
    foreach ($path in $paths) {
        $hashes[$path] = (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $root $path)).Hash
    }
    $head = (& git -C $root rev-parse HEAD).Trim()
    $dirty = [bool](& git -C $root status --porcelain)
    return [ordered]@{ git_head = $head; git_dirty = $dirty; sha256 = $hashes }
}

function AssertSameConfig($expected) {
    $now = ConfigFingerprint
    if ($now.git_head -ne $expected.git_head) { throw 'Git HEAD changed during the series.' }
    foreach ($path in $expected.sha256.Keys) {
        if ($now.sha256[$path] -ne $expected.sha256[$path]) { throw "Configuration changed during the series: $path" }
    }
}

$baseline = Preflight
$fingerprint = ConfigFingerprint
$completed = @{}
if ($Resume) {
    $reportRoot = Join-Path $root 'load-results'
    foreach ($scenario in $scenarios) {
        foreach ($vus in $levels) {
            $key = "$scenario/$vus"
            $matches = @()
            foreach ($folder in @(Get-ChildItem -LiteralPath $reportRoot -Directory)) {
                $manifestPath = Join-Path $folder.FullName 'manifest.json'
                if (-not (Test-Path -LiteralPath $manifestPath)) { continue }
                $saved = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
                if ($saved.status -eq 'complete' -and $saved.scenario -eq $scenario -and $saved.vus -eq $vus -and $saved.duration -eq $duration) {
                    foreach ($file in @('summary.json', 'metrics.ndjson', 'result.json', 'server-series.json')) {
                        if (-not (Test-Path -LiteralPath (Join-Path $folder.FullName $file))) { throw "Completed run $($saved.run_id) lacks $file" }
                    }
                    if ($saved.database_before.database -ne 'ai_buddy_load' -or $saved.database_before.notes -ne $baseline.notes -or $saved.database_before.events -ne $baseline.events -or $saved.database_before.users -ne 100 -or
                        $saved.database_after.database -ne 'ai_buddy_load' -or $saved.database_after.notes -ne $baseline.notes -or $saved.database_after.events -ne $baseline.events -or $saved.database_after.users -ne 100) {
                        throw "Completed run $($saved.run_id) has a different database baseline."
                    }
                    $matches += $saved
                }
            }
            if ($matches.Count -gt 1) { throw "Multiple completed runs found for $key; select a series manually." }
            if ($matches.Count -eq 1) { $completed[$key] = $matches[0] }
        }
    }
    $seenGap = $false
    foreach ($scenario in $scenarios) {
        foreach ($vus in $levels) {
            $key = "$scenario/$vus"
            if (-not $completed.ContainsKey($key)) { $seenGap = $true; continue }
            if ($seenGap) { throw "Completed run $key follows an incomplete run; cannot resume safely." }
            $saved = $completed[$key]
            $same = $saved.config.git_head -eq $fingerprint.git_head
            foreach ($path in $fingerprint.sha256.Keys) {
                $old = $saved.config.sha256.PSObject.Properties[$path]
                if (-not $old -or $old.Value -ne $fingerprint.sha256[$path]) { $same = $false }
            }
            if (-not $same -and -not $AllowChangedConfig) { throw "Code/config changed since $($saved.run_id). Resume requires -AllowChangedConfig; old and new runs are separate cohorts." }
        }
    }
    if ($AllowChangedConfig) { Write-Warning 'Completed runs use different code/config. New results must not be pooled with them as one comparable series.' }
}
Write-Host "Preflight OK: database=$($baseline.database), users=$($baseline.users), Prometheus target UP, Grafana healthy"
Write-Host "Git HEAD: $($fingerprint.git_head); dirty working tree: $($fingerprint.git_dirty)"
Write-Host "Baseline: notes=$($baseline.notes), events=$($baseline.events)"
foreach ($scenario in $scenarios) {
    foreach ($vus in $levels) {
        $key = "$scenario/$vus"
        if ($completed.ContainsKey($key)) { Write-Host "SKIP completed $key $($completed[$key].run_id)" }
        else { Write-Host "PLAN $scenario $vus VU $duration; pause $pauseSeconds s" }
    }
}
if (-not $Execute) {
    Write-Host "CHECK ONLY: no k6 runs, cleanup, or report directories created. MaxRuns=$MaxRuns. Use -Execute only after approval."
    return
}

$index = 0
$executed = 0
:scenarioLoop foreach ($scenario in $scenarios) {
    foreach ($vus in $levels) {
        $index++
        if ($completed.ContainsKey("$scenario/$vus")) { continue }
        if ($executed -ge $MaxRuns) { break scenarioLoop }
        AssertSameConfig $fingerprint
        $before = Preflight
        if ($before.notes -ne $baseline.notes -or $before.events -ne $baseline.events) { throw 'Baseline record counts changed; stop before the next run.' }
        $runId = '{0}-{1}-{2}' -f $scenario, (Get-Date -Format 'yyyyMMdd-HHmmss'), ([guid]::NewGuid().ToString('N').Substring(0, 8))
        $dir = Join-Path $root "load-results/$runId"
        $started = [DateTimeOffset]::UtcNow
        $manifest = [ordered]@{
            run_id = $runId; scenario = $scenario; vus = $vus; duration = $duration
            pause_seconds = $pauseSeconds; started_at_utc = $started.ToString('o')
            database_before = $before; config = $fingerprint; status = 'running'
            comparable_to_prior = (-not $AllowChangedConfig)
        }
        try {
            & (Join-Path $PSScriptRoot 'run-k6.ps1') -Scenario $scenario -VUs $vus -Duration $duration -RunId $runId
            $ended = [DateTimeOffset]::UtcNow
            AssertSameConfig $fingerprint
            Start-Sleep -Seconds 6 # allow the final Prometheus scrape
            & (Join-Path $PSScriptRoot 'collect-results.ps1') -RunId $runId -StartedAt $started -EndedAt $ended -PrometheusURL $promURL
            if ($scenario -eq 'mixed') {
                $preview = CleanupPreview $runId
                & docker @compose run --rm cleanup-load --run-id $runId --apply --expect-notes $preview.notes --expect-events $preview.events
                if ($LASTEXITCODE -ne 0) { throw "Cleanup failed for $runId" }
                $afterCleanup = CleanupPreview $runId
                if ($afterCleanup.notes -ne 0 -or $afterCleanup.events -ne 0) { throw "Cleanup incomplete for $runId" }
            }
            $after = DatabaseSnapshot
            if ($after.notes -ne $baseline.notes -or $after.events -ne $baseline.events) { throw "Baseline counts not restored after $runId" }
            $manifest.ended_at_utc = $ended.ToString('o')
            $manifest.database_after = $after
            $manifest.status = 'complete'
            $manifest | ConvertTo-Json -Depth 100 | Set-Content -LiteralPath (Join-Path $dir 'manifest.json') -Encoding utf8
            Write-Host "COMPLETE $index/9 $runId"
            $executed++
        } catch {
            $manifest.status = 'failed'
            $manifest.error = $_.Exception.Message
            $manifest.failed_at_utc = [DateTimeOffset]::UtcNow.ToString('o')
            $manifest.cleanup_pending = ($scenario -eq 'mixed')
            if (Test-Path -LiteralPath $dir) {
                $manifest.available_files = @(Get-ChildItem -LiteralPath $dir -File | Select-Object -ExpandProperty Name)
                $manifest | ConvertTo-Json -Depth 100 | Set-Content -LiteralPath (Join-Path $dir 'manifest.json') -Encoding utf8
            }
            throw
        }
        if ($index -lt 9 -and $executed -lt $MaxRuns) { Start-Sleep -Seconds $pauseSeconds }
    }
}

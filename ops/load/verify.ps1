$ErrorActionPreference = 'Stop'
$root = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$composeFile = Join-Path $root 'compose.load.yaml'
$envFile = Join-Path $root '.env.load'
if (-not (Test-Path -LiteralPath $envFile)) { throw 'Create .env.load from .env.load.example first.' }

$compose = @('compose', '--project-name', 'ai-buddy-load', '--file', $composeFile, '--env-file', $envFile)
$db = (& docker @compose exec -T postgres psql -U load_user -d ai_buddy_load -X -A -t -v ON_ERROR_STOP=1 -c 'SELECT current_database()').Trim()
if ($LASTEXITCODE -ne 0 -or $db -ne 'ai_buddy_load') { throw "Unexpected connected database: $db" }

$headFile = Get-ChildItem -LiteralPath (Join-Path $root 'apps/backend/migrations') -Filter '*.sql' | Where-Object { $_.Name -match '^\d+_.+\.sql$' } | Sort-Object { [int]($_.Name -split '_')[0] } | Select-Object -Last 1
if (-not $headFile) { throw 'No Goose migration files found.' }
$head = [int](($headFile.Name -split '_')[0])
$appliedText = (& docker @compose exec -T postgres psql -U load_user -d ai_buddy_load -X -A -t -v ON_ERROR_STOP=1 -c 'SELECT COALESCE(MAX(version_id),0) FROM goose_db_version WHERE is_applied').Trim()
if ($LASTEXITCODE -ne 0 -or [int]$appliedText -ne $head) { throw "Goose version $appliedText differs from repository head $head" }

$postgresId = (& docker @compose ps -q postgres).Trim()
if ($LASTEXITCODE -ne 0 -or -not $postgresId) { throw 'Load PostgreSQL container is missing.' }
$mounts = & docker inspect --format '{{json .Mounts}}' $postgresId | ConvertFrom-Json
$mount = ($mounts | Where-Object { $_.Destination -eq '/var/lib/postgresql/data' } | Select-Object -First 1).Name
if ($LASTEXITCODE -ne 0 -or $mount -ne 'ai-buddy-load_load_postgres_data') { throw "Unexpected PostgreSQL volume: $mount" }

$backendId = (& docker @compose ps -q backend).Trim()
if ($LASTEXITCODE -ne 0 -or -not $backendId) { throw 'Load backend container is missing.' }
$backendEnv = & docker inspect --format '{{json .Config.Env}}' $backendId | ConvertFrom-Json
$dbHost = $backendEnv | Where-Object { $_ -match '^DB_HOST=' } | Select-Object -First 1
$dbName = $backendEnv | Where-Object { $_ -match '^DB_NAME=' } | Select-Object -First 1
if ($dbHost -ne 'DB_HOST=postgres' -or $dbName -ne 'DB_NAME=ai_buddy_load') { throw "Unexpected backend DB settings: $dbHost, $dbName" }

$config = Get-Content -LiteralPath $envFile | Where-Object { $_ -match '^LOAD_BACKEND_PORT=' } | Select-Object -Last 1
$port = if ($config) { ($config -split '=',2)[1].Trim() } else { '18080' }
$health = Invoke-RestMethod -Uri "http://127.0.0.1:$port/health" -TimeoutSec 10
if ($health.status -ne 'ok') { throw 'Backend health smoke test failed.' }
$onboarding = Invoke-RestMethod -Uri "http://127.0.0.1:$port/api/onboarding?role=backend" -TimeoutSec 10
if ($null -eq $onboarding) { throw 'Backend onboarding smoke test failed.' }

Write-Host "OK: database=$db; Goose head=$head; volume=$mount; $dbHost; $dbName; API healthy on port $port"

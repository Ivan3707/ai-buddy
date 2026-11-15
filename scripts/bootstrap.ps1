param(
  [string]$ProjectName = 'ai-buddy',
  [string]$EnvFile = '.env',
  [string]$CredentialsDirectory = ''
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Continue' # Native Docker/Go progress on stderr is checked by exit code below.
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$composeFile = Join-Path $repoRoot 'compose.yaml'
$previous = @{}
$managed = @('DB_USER', 'DB_PASSWORD', 'DB_NAME', 'JWT_SECRET', 'WEB_PORT', 'BACKEND_PORT', 'POSTGRES_PORT', 'DB_HOST', 'DB_PORT', 'DB_SSLMODE', 'PGPASSWORD')
foreach ($name in $managed) { $previous[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }

function Safe-GoFailure([string]$detail) {
  foreach ($secret in @($env:DB_PASSWORD, $env:PGPASSWORD)) {
    if ($secret) { $detail = $detail.Replace($secret, '[REDACTED]') }
  }
  $safe = foreach ($line in ($detail -split "`r?`n")) {
    if ($line -match '(?i)postgres(?:ql)?://|(?:^|\s)password\s*=') {
      $state = [regex]::Match($line, 'SQLSTATE\s+[A-Z0-9]+').Value
      if ($state) { "[данные подключения скрыты] $state" } else { '[данные подключения скрыты]' }
    } elseif ($line.Trim()) { $line.Trim() }
  }
  return ($safe -join ' ').Trim()
}

try {
  foreach ($name in @('DB_USER', 'DB_PASSWORD', 'DB_NAME', 'JWT_SECRET', 'WEB_PORT', 'BACKEND_PORT', 'POSTGRES_PORT')) {
    [Environment]::SetEnvironmentVariable($name, $null, 'Process')
  }
  if ($env:OS -ne 'Windows_NT') { throw 'Этот bootstrap предназначен для Windows PowerShell.' }
  if ($ProjectName -notmatch '^[a-z0-9][a-z0-9_-]*$') { throw 'Некорректное имя Compose-проекта.' }
  if (-not [IO.Path]::IsPathRooted($EnvFile)) { $EnvFile = Join-Path $repoRoot $EnvFile }
  $EnvFile = [IO.Path]::GetFullPath($EnvFile)
  if (-not (Test-Path -LiteralPath $EnvFile -PathType Leaf)) { throw 'Создайте .env из .env.example и задайте секреты.' }
  foreach ($line in (Get-Content -LiteralPath $EnvFile -Encoding UTF8)) {
    if ($line -match '^\s*(?:#|$)') { continue }
    if ($line -notmatch '^([A-Z_][A-Z_0-9]*)=(.*)$') { throw 'Некорректная строка в .env.' }
    if ($managed -contains $Matches[1]) { [Environment]::SetEnvironmentVariable($Matches[1], $Matches[2], 'Process') }
  }
  if ([string]::IsNullOrWhiteSpace($env:DB_USER) -or $env:DB_USER -notmatch '^[a-zA-Z_][a-zA-Z_0-9]*$') { throw 'Задайте корректный DB_USER в .env.' }
  if ([string]::IsNullOrWhiteSpace($env:DB_NAME) -or $env:DB_NAME -notmatch '^[a-zA-Z_][a-zA-Z_0-9]*$') { throw 'Задайте корректный DB_NAME в .env.' }
  if ([string]::IsNullOrWhiteSpace($env:DB_PASSWORD) -or $env:DB_PASSWORD -in @('change_me', 'replace_with_your_database_password')) { throw 'Замените DB_PASSWORD в .env.' }
  if ([string]::IsNullOrWhiteSpace($env:JWT_SECRET) -or $env:JWT_SECRET.Length -lt 32 -or $env:JWT_SECRET -eq 'replace_with_random_secret_at_least_32_chars') { throw 'Задайте случайный JWT_SECRET длиной не менее 32 символов.' }
  if ([string]::IsNullOrWhiteSpace($env:POSTGRES_PORT)) { $env:POSTGRES_PORT = '5433' }
  $postgresPort = 0
  if (-not [int]::TryParse($env:POSTGRES_PORT, [ref]$postgresPort) -or $postgresPort -lt 1 -or $postgresPort -gt 65535) { throw 'Некорректный POSTGRES_PORT.' }
  if (-not (Get-Command docker -ErrorAction SilentlyContinue)) { throw 'Docker не найден.' }
  if (-not (Get-Command goose -ErrorAction SilentlyContinue)) { throw 'Goose не найден. Установите его по инструкции README.' }
  if (-not (Get-Command go -ErrorAction SilentlyContinue)) { throw 'Go не найден.' }
  docker compose version *> $null
  if ($LASTEXITCODE -ne 0) { throw 'Docker Compose недоступен.' }
  $composeArgs = @('-p', $ProjectName, '--env-file', $EnvFile, '-f', $composeFile)
  docker compose @composeArgs config --quiet *> $null
  if ($LASTEXITCODE -ne 0) { throw 'Конфигурация Compose некорректна.' }

  if (-not $CredentialsDirectory) { $CredentialsDirectory = Join-Path $repoRoot 'secrets' }
  $CredentialsDirectory = [IO.Path]::GetFullPath($CredentialsDirectory)
  $defaultSecretsDirectory = Join-Path $repoRoot 'secrets'
  if ($CredentialsDirectory.Equals($repoRoot, [StringComparison]::OrdinalIgnoreCase) -or
      ($CredentialsDirectory.StartsWith($repoRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase) -and
       -not $CredentialsDirectory.Equals($defaultSecretsDirectory, [StringComparison]::OrdinalIgnoreCase))) {
    throw 'Каталог учётных данных должен быть secrets/ в корне Git-репозитория или находиться вне репозитория.'
  }
  New-Item -ItemType Directory -Path $CredentialsDirectory -Force | Out-Null

  Push-Location $repoRoot
  try {
    Write-Host 'Запускаю PostgreSQL и жду готовности...'
    docker compose @composeArgs up -d --wait postgres *> $null
    if ($LASTEXITCODE -ne 0) { throw 'Не удалось запустить PostgreSQL.' }
    $state = docker compose @composeArgs exec -T postgres psql -U $env:DB_USER -d $env:DB_NAME -tAc "SELECT (to_regclass('public.goose_db_version') IS NOT NULL)::int || '|' || (to_regclass('public.users') IS NOT NULL)::int" 2>$null
    if ($LASTEXITCODE -ne 0) { throw 'Не удалось проверить состояние PostgreSQL.' }
    $state = ($state | Out-String).Trim()
    if ($state -eq '0|1') { throw 'В БД уже есть users без истории Goose; остановлено без миграции.' }
    if ($state -eq '1|1') {
      $version = docker compose @composeArgs exec -T postgres psql -U $env:DB_USER -d $env:DB_NAME -tAc 'SELECT COALESCE(MAX(version_id) FILTER (WHERE is_applied), 0) FROM goose_db_version' 2>$null
      # допустимы версии 1..N, где N — номер последней миграции в apps/backend/migrations; goose ниже доведёт БД до N
      $latest = (Get-ChildItem -LiteralPath (Join-Path $repoRoot 'apps/backend/migrations') -Filter '*.sql' | ForEach-Object { [int]($_.Name -split '_')[0] } | Measure-Object -Maximum).Maximum
      $current = ($version | Out-String).Trim()
      if ($LASTEXITCODE -ne 0 -or $current -notmatch '^\d+$' -or [int]$current -lt 1 -or [int]$current -gt $latest) { throw "История Goose вне диапазона 1–$latest; существующую БД не мигрирую." }
    } elseif ($state -ne '0|0') { throw 'Неожиданное состояние схемы PostgreSQL.' }

    Write-Host 'Применяю миграции Goose...'
    $env:PGPASSWORD = $env:DB_PASSWORD
    $dsn = "host=127.0.0.1 port=$postgresPort user=$($env:DB_USER) dbname=$($env:DB_NAME) sslmode=disable"
    goose -dir (Join-Path $repoRoot 'apps/backend/migrations') postgres $dsn up *> $null
    if ($LASTEXITCODE -ne 0) { throw 'Миграции Goose не выполнены.' }

    $env:DB_HOST = '127.0.0.1'
    $env:DB_PORT = [string]$postgresPort
    $env:DB_SSLMODE = 'disable'
    Push-Location (Join-Path $repoRoot 'apps/backend')
    try {
      Write-Host 'Инициализирую каталог чек-листов...'
      $catalogOutput = go run ./cmd/init-catalog 2>&1 | Out-String
      if ($LASTEXITCODE -ne 0) { throw ('Инициализация каталога не выполнена: ' + (Safe-GoFailure $catalogOutput)) }
      $credentialsFile = Join-Path $CredentialsDirectory ('hr-' + (Get-Date -Format 'yyyyMMdd-HHmmss') + '-' + [guid]::NewGuid().ToString('N') + '.json')
      Write-Host 'Синхронизирую сотрудников Mock HR...'
      $syncOutput = go run ./cmd/sync-hr -snapshot (Join-Path $repoRoot 'data/corporate_employees.json') -credentials $credentialsFile 2>&1 | Out-String
      if ($LASTEXITCODE -ne 0) { throw ('Mock HR Sync не выполнен; прежние файлы учётных данных сохранены. Причина: ' + (Safe-GoFailure $syncOutput)) }
    } finally { Pop-Location }

    $credentials = Get-Content -LiteralPath $credentialsFile -Raw -Encoding UTF8 | ConvertFrom-Json
    $issued = @($credentials).Count
    if ($issued -gt 0) { Write-Host "Выдано новых учётных данных: $issued. Приватный файл: $credentialsFile" }
    else { Write-Host 'Новых учётных данных нет. Прежний файл не изменён; новый файл содержит пустой список.' }
    Write-Host 'Запускаю приложение и жду готовности сервисов...'
    docker compose @composeArgs up -d --build --wait *> $null
    if ($LASTEXITCODE -ne 0) { throw 'Приложение не стало готовым. Проверьте docker compose ps.' }
    $running = @(docker compose @composeArgs ps --services --status running)
    if ($LASTEXITCODE -ne 0 -or @($running | Where-Object { $_ -in @('postgres', 'ai', 'backend', 'web') }).Count -ne 4) { throw 'Не все сервисы запущены.' }
    Write-Host 'Bootstrap завершён. Web и Backend готовы.'
  } finally { Pop-Location }
} catch {
  Write-Error ('Bootstrap остановлен: ' + $_.Exception.Message)
  exit 1
} finally {
  foreach ($name in $managed) { [Environment]::SetEnvironmentVariable($name, $previous[$name], 'Process') }
}

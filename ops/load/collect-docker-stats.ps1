
param(
    [int]$IntervalSeconds = 5,
    [string]$Output = "load-results/docker-stats.csv"
)

$ErrorActionPreference = "Stop"

$directory = Split-Path -Parent $Output

if ($directory -and -not (Test-Path $directory)) {
    New-Item -ItemType Directory -Path $directory -Force |
        Out-Null
}

$containers = @(
    "ai-buddy-load-backend-1",
    "ai-buddy-load-postgres-1",
    "ai-buddy-load-ai-1"
)

"timestamp,container,cpu_percent,memory_mib" |
    Set-Content -Path $Output -Encoding UTF8

Write-Host "Collecting Docker metrics. Press Ctrl+C to stop."

while ($true) {
    $timestamp = [DateTimeOffset]::UtcNow.ToString("o")

    $stats = docker stats --no-stream `
        --format "{{.Name}}|{{.CPUPerc}}|{{.MemUsage}}"

    foreach ($line in $stats) {
        $parts = $line -split "\|"

        if ($parts.Count -ne 3) {
            continue
        }

        $name = $parts[0]

        if ($name -notin $containers) {
            continue
        }

        $cpu = [double]::Parse(
            $parts[1].TrimEnd("%"),
            [cultureinfo]::InvariantCulture
        )

        $memory = ($parts[2] -split "/")[0].Trim()

        if ($memory -match "^([\d.]+)(KiB|MiB|GiB)$") {
            $value = [double]::Parse(
                $Matches[1],
                [cultureinfo]::InvariantCulture
            )

            $factor = switch ($Matches[2]) {
                "KiB" { 1 / 1024 }
                "MiB" { 1 }
                "GiB" { 1024 }
            }

            $memoryMiB = $value * $factor

            "$timestamp,$name,$cpu,$memoryMiB" |
                Add-Content -Path $Output
        }
    }

    Start-Sleep -Seconds $IntervalSeconds
}

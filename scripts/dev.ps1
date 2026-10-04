[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet('init', 'dev', 'up', 'down', 'logs', 'status', 'restart', 'migrate-up', 'migrate-version', 'frontend-check', 'backend-check')]
    [string]$Action = 'dev',
    [ValidateSet('dev', 'built')]
    [string]$Mode = 'dev',
    [ValidateSet('frontend', 'backend', 'postgres', 'mailpit', 'migrate')]
    [string]$Service
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot

if ($Action -eq 'init') {
    $envPath = Join-Path $projectRoot '.env'
    if (Test-Path -LiteralPath $envPath) {
        Write-Host 'Root .env already exists; kept unchanged.'
        exit 0
    }
    $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try {
        $secretBytes = New-Object byte[] 32
        $rng.GetBytes($secretBytes)
        $accessSecret = ([BitConverter]::ToString($secretBytes)).Replace('-', '').ToLowerInvariant()
        $rng.GetBytes($secretBytes)
        $refreshSecret = ([BitConverter]::ToString($secretBytes)).Replace('-', '').ToLowerInvariant()
    } finally {
        $rng.Dispose()
    }
    $template = [IO.File]::ReadAllText((Join-Path $projectRoot '.env.example'))
    $template = $template.Replace('GENERATE_ACCESS_SECRET', $accessSecret).Replace('GENERATE_REFRESH_SECRET', $refreshSecret)
    [IO.File]::WriteAllText($envPath, $template, (New-Object System.Text.UTF8Encoding $false))
    Write-Host 'Created root .env with independent random JWT secrets.'
    exit 0
}

$composeArgs = @('compose', '--project-directory', $projectRoot, '-f', (Join-Path $projectRoot 'compose.yaml'))
if ($Action -eq 'dev' -or ($Action -ne 'up' -and $Mode -eq 'dev')) {
    $composeArgs += @('-f', (Join-Path $projectRoot 'compose.dev.yaml'))
}

switch ($Action) {
    'dev' { $composeArgs += @('up', '--build', '-d', '--wait', '--wait-timeout', '300') }
    'up' { $composeArgs += @('up', '--build', '-d', '--wait', '--wait-timeout', '300') }
    'down' { $composeArgs += @('down', '--remove-orphans') }
    'logs' {
        $composeArgs += @('logs', '-f', '--tail', '100')
        if ($Service) { $composeArgs += $Service }
    }
    'status' { $composeArgs += 'ps' }
    'restart' {
        if (-not $Service) { throw 'Use -Service to select the container to restart.' }
        if ($Service -eq 'migrate') { throw 'Use migrate-up to apply migrations.' }
        $composeArgs += @('restart', $Service)
    }
    'migrate-up' { $composeArgs += @('run', '--rm', 'migrate') }
    'migrate-version' {
        $composeArgs += @('run', '--rm', 'migrate', 'exec migrate -path /migrations -database "postgres://postgres:5432/$PGDATABASE?sslmode=disable" version')
    }
    'frontend-check' {
        $composeArgs = @('compose', '--project-directory', $projectRoot, '-f', (Join-Path $projectRoot 'compose.yaml'), '-f', (Join-Path $projectRoot 'compose.dev.yaml'))
        $composeArgs += @('run', '--build', '--rm', '--no-deps', 'frontend', 'sh', '-ec', 'npm ci --no-audit --no-fund && npm run lint && npm run build')
    }
    'backend-check' {
        $composeArgs = @('compose', '--project-directory', $projectRoot, '-f', (Join-Path $projectRoot 'compose.yaml'), '-f', (Join-Path $projectRoot 'compose.dev.yaml'))
        $composeArgs += @('run', '--build', '--rm', '--no-deps', 'backend', 'sh', '-ec', 'go mod verify && go test ./... && go vet ./... && go build -o /tmp/api-check ./cmd/api')
    }
}

& docker @composeArgs
exit $LASTEXITCODE

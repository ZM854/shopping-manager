[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet('init', 'dev', 'up', 'down', 'logs', 'status', 'restart', 'migrate-up', 'migrate-version', 'frontend-check', 'backend-check', 'frontend-test', 'frontend-coverage', 'backend-test', 'backend-coverage', 'backend-integration', 'integration-coverage', 'media-check', 'e2e', 'e2e-up', 'e2e-stop')]
    [string]$Action = 'dev',
    [ValidateSet('dev', 'built')]
    [string]$Mode = 'dev',
    [ValidateSet('frontend', 'backend', 'postgres', 'mailpit', 'migrate', 'storage')]
    [string]$Service
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot

$testActions = @{
    'frontend-test' = 'frontend'
    'frontend-coverage' = 'frontend-coverage'
    'backend-test' = 'backend'
    'backend-coverage' = 'backend-coverage'
    'backend-integration' = 'integration'
    'integration-coverage' = 'integration-coverage'
    'media-check' = 'media'
    'e2e' = 'e2e'
    'e2e-up' = 'e2e-up'
    'e2e-stop' = 'e2e-stop'
}
if ($testActions.ContainsKey($Action)) {
    & (Join-Path $PSScriptRoot 'test.ps1') $testActions[$Action]
    exit $LASTEXITCODE
}

if ($Action -eq 'init') {
    & node (Join-Path $PSScriptRoot 'init-env.mjs')
    exit $LASTEXITCODE
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

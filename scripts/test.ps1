[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet('frontend', 'frontend-coverage', 'backend', 'backend-coverage', 'integration', 'test-db-up')]
    [string]$Action = 'frontend'
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot

if ($Action -in @('integration', 'test-db-up')) {
    $testComposeArgs = @('compose', '--project-directory', $projectRoot, '-f', (Join-Path $projectRoot 'compose.test.yaml'))
    if ($Action -eq 'test-db-up') {
        & docker @testComposeArgs up -d --wait --wait-timeout 90 postgres-test
    } else {
        # run возвращает код именно тестов; рабочие контейнеры не затрагиваются.
        & docker @testComposeArgs run --build --rm backend-test
    }
    exit $LASTEXITCODE
}

if ($Action -in @('frontend', 'frontend-coverage')) {
    $npmScript = if ($Action -eq 'frontend-coverage') { 'test:coverage' } else { 'test' }
    & npm.cmd --prefix (Join-Path $projectRoot 'frontend') run $npmScript
    exit $LASTEXITCODE
}

$backendRoot = Join-Path $projectRoot 'backend'
$env:GOMODCACHE = Join-Path $backendRoot '.tools\go\pkg\mod'
$env:GOCACHE = Join-Path $backendRoot '.cache\go-build'
Push-Location $backendRoot
try {
    if ($Action -eq 'backend-coverage') {
        New-Item -ItemType Directory -Force -Path '.cache' | Out-Null
        & go test ./... '-coverprofile=.cache/coverage.out'
    } else {
        & go test ./...
    }
    $testExitCode = $LASTEXITCODE
} finally {
    Pop-Location
}
exit $testExitCode

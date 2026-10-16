[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet('frontend', 'frontend-coverage', 'backend', 'backend-coverage', 'integration', 'integration-coverage', 'test-db-up', 'e2e', 'e2e-up', 'e2e-stop')]
    [string]$Action = 'frontend'
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot

if ($Action -in @('e2e', 'e2e-up', 'e2e-stop')) {
    & node (Join-Path $PSScriptRoot 'e2e-env.mjs')
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    $e2eComposeArgs = @(
        'compose', '--env-file', (Join-Path $projectRoot '.env.e2e.local'),
        '--project-directory', $projectRoot,
        '-f', (Join-Path $projectRoot 'compose.e2e.yaml')
    )
    if ($Action -eq 'e2e-stop') {
        & docker @e2eComposeArgs stop
        exit $LASTEXITCODE
    }
    & docker @e2eComposeArgs up --build -d --wait --wait-timeout 300
    if ($LASTEXITCODE -ne 0 -or $Action -eq 'e2e-up') { exit $LASTEXITCODE }
    & npm.cmd --prefix (Join-Path $projectRoot 'frontend') run test:e2e
    exit $LASTEXITCODE
}

if ($Action -in @('integration', 'integration-coverage', 'test-db-up')) {
    $testComposeArgs = @('compose', '--project-directory', $projectRoot, '-f', (Join-Path $projectRoot 'compose.test.yaml'))
    if ($Action -eq 'test-db-up') {
        & docker @testComposeArgs up -d --wait --wait-timeout 90 postgres-test
    } elseif ($Action -eq 'integration-coverage') {
        $coveragePath = Join-Path $projectRoot 'backend/.cache'
        New-Item -ItemType Directory -Force -Path $coveragePath | Out-Null
        & docker @testComposeArgs run --build --rm --volume "${coveragePath}:/coverage" backend-test go test -count=1 -tags=integration '-coverpkg=./internal/...' '-coverprofile=/coverage/integration-coverage.out' ./...
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

<#
.SYNOPSIS
    Lanzamiento formal de release para hitos acumulativos de Axiom.
.DESCRIPTION
    Verifica estado limpio del repositorio en main, valida tests unitarios, crea un tag anotado semver y lo sube a GitHub
    para disparar el workflow de GoReleaser (.github/workflows/release.yml).
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true, HelpMessage = "Versión semver a publicar (ej. v3.5.0)")]
    [ValidatePattern('^v\d+\.\d+\.\d+$')]
    [string]$Version,

    [string]$Message = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

Write-Host "=== Axiom Milestone Release Pipeline ===" -ForegroundColor Cyan
Write-Host "Versión solicitada: $Version" -ForegroundColor Yellow

$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $RepoRoot

# 1. Comprobar rama actual
$CurrentBranch = (git branch --show-current).Trim()
if ($CurrentBranch -ne "main") {
    Write-Error "Solo se pueden generar releases oficiales desde la rama 'main'. Rama actual: $CurrentBranch"
    exit 1
}

# 2. Comprobar árbol Git limpio
$Status = (git status -s)
if ($Status) {
    Write-Error "El repositorio tiene cambios pendientes sin commitear. Limpia o commitea antes de publicar una release."
    exit 1
}

# 3. Actualizar la versión por defecto en cmd/axiom/main.go y cmd/axiom/version_test.go si difiere
$MainGoPath = Join-Path $RepoRoot "cmd/axiom/main.go"
$VersionTestPath = Join-Path $RepoRoot "cmd/axiom/version_test.go"
$FilesToCommit = @()

if (Test-Path $MainGoPath) {
    $MainContent = Get-Content -Raw -Encoding UTF8 $MainGoPath
    $UpdatedMain = $MainContent -replace '(?m)^var version = "v[^"]*"', "var version = `"$Version`""
    $UpdatedMain = $UpdatedMain -replace '(?m)\/\/ A compilation without injection reports "[^"]*"', "// A compilation without injection reports `"$Version`""
    if ($MainContent -ne $UpdatedMain) {
        Set-Content -Path $MainGoPath -Value $UpdatedMain -Encoding UTF8 -NoNewline
        $FilesToCommit += $MainGoPath
    }
}

if (Test-Path $VersionTestPath) {
    $TestContent = Get-Content -Raw -Encoding UTF8 $VersionTestPath
    $UpdatedTest = $TestContent -replace '(?m)if version != "v[^"]*"', "if version != `"$Version`""
    $UpdatedTest = $UpdatedTest -replace '(?m)want %q", version, "v[^"]*"', "want %q`", version, `"$Version`""
    if ($TestContent -ne $UpdatedTest) {
        Set-Content -Path $VersionTestPath -Value $UpdatedTest -Encoding UTF8 -NoNewline
        $FilesToCommit += $VersionTestPath
    }
}

if ($FilesToCommit.Count -gt 0) {
    Write-Host "[1/5] Actualizando versión por defecto en cmd/axiom a $Version..." -ForegroundColor Yellow
    git add $FilesToCommit
    git commit -m "chore(release): bump default version to $Version in cmd/axiom"
    Write-Host "      Commit registrado en main." -ForegroundColor Green
} else {
    Write-Host "[1/5] La versión por defecto en cmd/axiom ya coincide con $Version." -ForegroundColor Gray
}

# 4. Validar tests
Write-Host "[2/5] Ejecutando batería de pruebas antes de taggear..." -ForegroundColor Yellow
go test ./internal/app ./cmd/axiom -count=1
if ($LASTEXITCODE -ne 0) {
    Write-Error "Las pruebas unitarias fallaron. Abortando lanzamiento de release."
    exit $LASTEXITCODE
}

# 5. Asegurar push de main con el nuevo commit antes de taggear
Write-Host "[3/5] Sincronizando rama main con origin..." -ForegroundColor Yellow
git push origin main

# 6. Crear tag
if ([string]::IsNullOrWhiteSpace($Message)) {
    $Message = "Release $Version: Hito acumulativo de Axiom"
}

Write-Host "[4/5] Creando tag anotado $Version..." -ForegroundColor Yellow
git tag -a $Version -m "$Message"

# 7. Push tag a origin
Write-Host "[5/5] Subiendo tag a GitHub (esto disparará release.yml en GitHub Actions)..." -ForegroundColor Yellow
git push origin $Version

Write-Host "=== Tag $Version publicado exitosamente ===" -ForegroundColor Green
Write-Host "Monitorea la compilación de GoReleaser en: https://github.com/IGutierrezZ/axiom/actions/workflows/release.yml" -ForegroundColor Cyan

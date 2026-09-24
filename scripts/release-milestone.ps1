<#
.SYNOPSIS
    Lanzamiento formal de release para hitos acumulativos de Axiom.
.DESCRIPTION
    Verifica estado limpio del repositorio en main, valida tests unitarios, crea un tag anotado semver y lo sube a GitHub
    para disparar el workflow de GoReleaser (.github/workflows/release.yml).
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true, HelpMessage = "Versión semver a publicar (ej. v0.2.0)")]
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

# 3. Validar tests
Write-Host "[1/3] Ejecutando batería de pruebas antes de taggear..." -ForegroundColor Yellow
go test ./internal/app ./cmd/axiom -count=1
if ($LASTEXITCODE -ne 0) {
    Write-Error "Las pruebas unitarias fallaron. Abortando lanzamiento de release."
    exit $LASTEXITCODE
}

# 4. Crear tag
if ([string]::IsNullOrWhiteSpace($Message)) {
    $Message = "Release $Version: Hito acumulativo de Axiom"
}

Write-Host "[2/3] Creando tag anotado $Version..." -ForegroundColor Yellow
git tag -a $Version -m "$Message"

# 5. Push tag a origin
Write-Host "[3/3] Subiendo tag a GitHub (esto disparará release.yml en GitHub Actions)..." -ForegroundColor Yellow
git push origin $Version

Write-Host "=== Tag $Version publicado exitosamente ===" -ForegroundColor Green
Write-Host "Monitorea la compilación de GoReleaser en: https://github.com/IGutierrezZ/axiom/actions/workflows/release.yml" -ForegroundColor Cyan

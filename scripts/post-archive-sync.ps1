<#
.SYNOPSIS
    Sincronización local tras archivar un incremento en Axiom.
.DESCRIPTION
    Alinea la rama local de main, recompila el binario axiom y ejecuta axiom sync --scope=workspace
    para dejar el entorno local 100% actualizado con los cambios recién archivados.
#>
[CmdletBinding()]
param(
    [switch]$SkipPull
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

Write-Host "=== Axiom Post-Archive Local Sync ===" -ForegroundColor Cyan

$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $RepoRoot

if (-not $SkipPull) {
    Write-Host "[1/3] Sincronizando con rama main remota..." -ForegroundColor Yellow
    git checkout main
    git pull origin main
} else {
    Write-Host "[1/3] Omitiendo git pull (SkipPull activo)..." -ForegroundColor DarkGray
}

Write-Host "[2/3] Recompilando e instalando binario axiom en PATH..." -ForegroundColor Yellow
go install ./cmd/axiom
if ($LASTEXITCODE -ne 0) {
    Write-Error "Fallo al compilar e instalar axiom."
    exit $LASTEXITCODE
}

Write-Host "[3/3] Ejecutando sincronización de agentes por workspace..." -ForegroundColor Yellow
axiom sync --scope=workspace
if ($LASTEXITCODE -ne 0) {
    Write-Error "Fallo en axiom sync --scope=workspace."
    exit $LASTEXITCODE
}

Write-Host "=== Entorno local de Axiom actualizado con éxito ===" -ForegroundColor Green
axiom version

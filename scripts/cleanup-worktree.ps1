# scripts/cleanup-worktree.ps1 — Desmantelamiento limpio y seguro de git worktrees y ramas locales post-merge (ODD-5.7).
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true, Position=0, HelpMessage="Ruta al directorio del worktree a eliminar")]
    [string]$WorktreePath,

    [Parameter(Mandatory=$false, Position=1, HelpMessage="Nombre de la rama local asociada para eliminar")]
    [string]$BranchName = "",

    [Parameter(Mandatory=$false, HelpMessage="Fuerza la eliminación aunque haya cambios sin guardar o rama sin mergear")]
    [switch]$Force
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $WorktreePath)) {
    Write-Warning "El directorio del worktree '$WorktreePath' no existe en disco. Ejecutando prune de referencias..."
    git worktree prune
    exit 0
}

# Comprobar cambios sin commitear en el worktree
try {
    $dirty = git -C $WorktreePath status --porcelain 2>$null
    if ($dirty -and (-not $Force)) {
        Write-Error "El worktree en '$WorktreePath' contiene cambios sin commitear:`n$dirty`nUsa -Force para forzar la eliminación."
        exit 1
    }
} catch {
    # Ignorar errores de detección
}

Write-Host "Desmontando worktree en '$WorktreePath'..." -ForegroundColor Cyan
if ($Force) {
    git worktree remove --force $WorktreePath
} else {
    git worktree remove $WorktreePath
}

git worktree prune

if ($BranchName) {
    Write-Host "Eliminando rama local '$BranchName'..." -ForegroundColor Cyan
    if ($Force) {
        git branch -D $BranchName
    } else {
        try {
            git branch -d $BranchName
        } catch {
            Write-Warning "No se pudo borrar la rama '$BranchName' con -d (quizá no está mergeada en HEAD). Usa -Force para forzar."
        }
    }
}

Write-Host "[OK] Worktree '$WorktreePath' desmantelado exitosamente." -ForegroundColor Green

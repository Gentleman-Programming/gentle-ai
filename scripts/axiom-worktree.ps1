#Requires -Version 5.1
<#
.SYNOPSIS
    Gestión unificada de worktrees y ciclo de vida de PRs para Axiom.

.DESCRIPTION
    Una unidad de trabajo = un worktree aislado + una rama (feat/<slug>, inc/<slug>, etc.)
    creada desde origin/main actualizado. El cambio se envía mediante Pull Request hacia main
    y, tras el merge, se limpian el worktree y la rama local.

.USAGE
    scripts/axiom-worktree.ps1 new <slug> [-Prefix feat|inc|fix|refactor]
    scripts/axiom-worktree.ps1 pr <slug> [-Prefix feat|inc|fix|refactor]
    scripts/axiom-worktree.ps1 done <slug> [-Prefix feat|inc|fix|refactor] [-Force]
#>
param(
    [Parameter(Position = 0, Mandatory = $true)]
    [ValidateSet('new', 'pr', 'done')]
    [string]$Verb,

    [Parameter(Position = 1, Mandatory = $true)]
    [string]$Slug,

    [Parameter(Position = 2, Mandatory = $false)]
    [ValidateSet('feat', 'inc', 'fix', 'refactor')]
    [string]$Prefix = 'feat',

    [Parameter(Mandatory = $false)]
    [switch]$Force
)

$ErrorActionPreference = 'Continue'

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot  = Split-Path -Parent $scriptDir

# La raíz de worktrees vive junto al checkout PRINCIPAL del repo. Al ejecutar
# desde un worktree, el padre de $repoRoot ya es la carpeta de worktrees, así
# que se deriva la raíz principal desde el git common dir.
$commonDir = git -C $repoRoot rev-parse --path-format=absolute --git-common-dir 2>$null
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($commonDir)) {
    $cd = (git -C $repoRoot rev-parse --git-common-dir 2>$null)
    if ($cd) {
        $commonDir = (Resolve-Path -LiteralPath (Join-Path $repoRoot $cd.Trim())).Path
    }
}
if ([string]::IsNullOrWhiteSpace($commonDir)) {
    $commonDir = Join-Path $repoRoot '.git'
}
$commonDir = $commonDir.Trim()
$mainRoot  = Split-Path -Parent $commonDir
$wtRoot    = Join-Path -Path (Split-Path -Parent $mainRoot) -ChildPath 'axiom-wt'
$wtPath    = Join-Path -Path $wtRoot -ChildPath $Slug

function Info { param($msg) Write-Host $msg -ForegroundColor Cyan }
function Ok   { param($msg) Write-Host $msg -ForegroundColor Green }
function Warn { param($msg) Write-Host "AVISO: $msg" -ForegroundColor Yellow }
function Fail { param($msg) Write-Host "ERROR: $msg" -ForegroundColor Red; exit 1 }

# --- Validaciones comunes ---
if ($Slug -notmatch '^[a-z0-9]+(-[a-z0-9]+)*$') {
    Fail "Slug inválido '$Slug'. Usa kebab-case en minúsculas (ej. worktree-and-pr-governance)."
}

git -C $repoRoot rev-parse --is-inside-work-tree 2>$null | Out-Null
if ($LASTEXITCODE -ne 0) { Fail "No es un repositorio git: $repoRoot" }

# Resolución inteligente de rama: si ya existe una rama con prefijo común para este slug, la reutiliza
$branch = "$Prefix/$Slug"
if ($Verb -ne 'new') {
    $candidates = @("$Prefix/$Slug", "feat/$Slug", "inc/$Slug", "fix/$Slug", "refactor/$Slug") | Select-Object -Unique
    foreach ($cand in $candidates) {
        git -C $repoRoot show-ref --verify --quiet "refs/heads/$cand" 2>$null
        if ($LASTEXITCODE -eq 0) {
            $branch = $cand
            break
        }
    }
}

switch ($Verb) {

    'new' {
        if (Test-Path -LiteralPath $wtPath) {
            Fail "Ya existe un worktree en: $wtPath"
        }

        git -C $repoRoot show-ref --verify --quiet "refs/heads/$branch" 2>$null
        if ($LASTEXITCODE -eq 0) {
            Fail "La rama $branch ya existe localmente. Elige otro slug o elimina la rama."
        }

        Info "Actualizando origin/main..."
        git -C $repoRoot fetch origin main
        if ($LASTEXITCODE -ne 0) {
            Warn "El fetch de origin/main falló (¿sin conexión?); se usa la referencia local como base."
        }

        $base = 'origin/main'
        git -C $repoRoot show-ref --verify --quiet 'refs/remotes/origin/main' 2>$null
        if ($LASTEXITCODE -ne 0) { $base = 'main' }

        New-Item -ItemType Directory -Force -Path $wtRoot | Out-Null
        git -C $repoRoot worktree add -b $branch $wtPath $base
        if ($LASTEXITCODE -ne 0) {
            Fail "git worktree add falló. Revisa el error anterior."
        }

        Ok "Worktree creado: $wtPath"
        Ok "Rama activa: $branch (base: $base)"
        Info "Trabaja dentro de ese directorio. Al verificar y pasar pruebas, ejecuta: scripts/axiom-worktree.ps1 pr $Slug"
    }

    'pr' {
        if (-not (Test-Path -LiteralPath $wtPath)) {
            Fail "No existe el worktree en: $wtPath"
        }

        $dirty = git -C $wtPath status --porcelain
        if ($dirty) {
            Fail "El worktree tiene cambios sin commitear. Haz commit de todo antes de crear el PR:`n$dirty"
        }

        Info "Subiendo $branch a origin..."
        git -C $wtPath push -u origin $branch
        if ($LASTEXITCODE -ne 0) {
            Fail "git push falló. Revisa tu conexión y permisos sobre origin."
        }

        $remoteUrl = (git -C $wtPath remote get-url origin 2>$null).Trim()
        $remoteUrl = $remoteUrl -replace '\.git$', ''
        $compareUrl = "$remoteUrl/compare/main...$branch" + '?expand=1'

        $gh = Get-Command gh -ErrorAction SilentlyContinue
        if ($gh) {
            gh auth status 2>$null
            if ($LASTEXITCODE -eq 0) {
                Info "Creando el Pull Request mediante gh CLI..."
                Push-Location $wtPath
                try {
                    & gh pr create --base main --fill
                    if ($LASTEXITCODE -eq 0) {
                        Ok "Pull Request creado exitosamente."
                        return
                    }
                }
                finally {
                    Pop-Location
                }
            }
            else {
                Warn "gh está instalado pero no autenticado ('gh auth login'). Abre el PR manualmente:"
            }
        }
        else {
            Warn "gh CLI no está disponible. Abre el PR manualmente en GitHub:"
        }
        Info $compareUrl
    }

    'done' {
        $currentBranch = (git -C $repoRoot rev-parse --abbrev-ref HEAD 2>$null).Trim()
        if ($currentBranch -eq $branch) {
            Fail "La rama $branch está chequeada en el checkout principal. Cambia a main antes de eliminar."
        }

        if (Test-Path -LiteralPath $wtPath) {
            Info "Eliminando el worktree en $wtPath..."
            if ($Force) {
                git -C $repoRoot worktree remove --force $wtPath
            } else {
                git -C $repoRoot worktree remove $wtPath
            }
            if ($LASTEXITCODE -ne 0) {
                Fail "No se pudo eliminar el worktree (¿cambios sin commitear?). Usa -Force o revísalo manualmente."
            }
        }
        else {
            Info "El directorio del worktree $wtPath no existe; continuando con limpieza de rama..."
        }

        # Sincronizar main si estamos en él en el checkout principal
        $currentBranch = (git -C $repoRoot rev-parse --abbrev-ref HEAD 2>$null).Trim()
        if ($currentBranch -eq 'main') {
            Info "Actualizando main local con origin/main..."
            git -C $repoRoot fetch origin main
            git -C $repoRoot merge --ff-only origin/main
            if ($LASTEXITCODE -eq 0) {
                Ok "main local actualizado correctamente."
            } else {
                Warn "main local no se pudo actualizar vía fast-forward. Realiza pull manual."
            }
        }

        Info "Eliminando rama local $branch..."
        if ($Force) {
            git -C $repoRoot branch -D $branch 2>$null
        } else {
            git -C $repoRoot branch -d $branch 2>$null
        }
        if ($LASTEXITCODE -ne 0) {
            Warn "No se pudo borrar $branch localmente con -d (¿aún no mergeada?). Usa -Force si ya está integrada."
        } else {
            Ok "Rama local $branch eliminada."
        }

        git -C $repoRoot worktree prune
        Ok "Ciclo de trabajo '$Slug' desmantelado y entorno limpio."
    }
}

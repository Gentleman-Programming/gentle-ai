#!/usr/bin/env bash
# scripts/axiom-worktree.sh — Gestión unificada de worktrees y ciclo de vida de PRs para Axiom.
set -euo pipefail

VERB="${1:-}"
SLUG="${2:-}"
PREFIX="${3:-feat}"
FORCE=0

usage() {
    echo "Uso: $0 {new|pr|done} <slug> [feat|inc|fix|refactor] [--force]"
    exit 1
}

if [ -z "$VERB" ] || [ -z "$SLUG" ]; then
    usage
fi

for arg in "$@"; do
    if [ "$arg" = "--force" ] || [ "$arg" = "-f" ]; then
        FORCE=1
    fi
done

case "$VERB" in
    new|pr|done) ;;
    *) echo "ERROR: Verbo desconocido '$VERB'. Usa 'new', 'pr' o 'done'." >&2; exit 1 ;;
esac

if ! [[ "$SLUG" =~ ^[a-z0-9]+(-[a-z0-9]+)*$ ]]; then
    echo "ERROR: Slug inválido '$SLUG'. Usa kebab-case en minúsculas (ej. worktree-and-pr-governance)." >&2
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

if ! git -C "$REPO_ROOT" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    echo "ERROR: No es un repositorio git: $REPO_ROOT" >&2
    exit 1
fi

COMMON_DIR="$(git -C "$REPO_ROOT" rev-parse --path-format=absolute --git-common-dir 2>/dev/null || true)"
if [ -z "$COMMON_DIR" ]; then
    CD_REL="$(git -C "$REPO_ROOT" rev-parse --git-common-dir 2>/dev/null || true)"
    if [ -n "$CD_REL" ]; then
        COMMON_DIR="$(cd "$REPO_ROOT/$CD_REL" && pwd)"
    else
        COMMON_DIR="$REPO_ROOT/.git"
    fi
fi

MAIN_ROOT="$(cd "$COMMON_DIR/.." && pwd)"
PARENT_DIR="$(cd "$MAIN_ROOT/.." && pwd)"
WT_ROOT="$PARENT_DIR/axiom-wt"
WT_PATH="$WT_ROOT/$SLUG"

BRANCH="$PREFIX/$SLUG"
if [ "$VERB" != "new" ]; then
    for cand in "$PREFIX/$SLUG" "feat/$SLUG" "inc/$SLUG" "fix/$SLUG" "refactor/$SLUG"; do
        if git -C "$REPO_ROOT" show-ref --verify --quiet "refs/heads/$cand"; then
            BRANCH="$cand"
            break
        fi
    done
fi

case "$VERB" in
    new)
        if [ -d "$WT_PATH" ]; then
            echo "ERROR: Ya existe un worktree en: $WT_PATH" >&2
            exit 1
        fi

        if git -C "$REPO_ROOT" show-ref --verify --quiet "refs/heads/$BRANCH"; then
            echo "ERROR: La rama $BRANCH ya existe localmente. Elige otro slug o elimina la rama." >&2
            exit 1
        fi

        echo "[INFO] Actualizando origin/main..."
        git -C "$REPO_ROOT" fetch origin main 2>/dev/null || echo "[AVISO] Fetch de origin/main falló; usando base local."

        BASE="origin/main"
        if ! git -C "$REPO_ROOT" show-ref --verify --quiet "refs/remotes/origin/main"; then
            BASE="main"
        fi

        mkdir -p "$WT_ROOT"
        git -C "$REPO_ROOT" worktree add -b "$BRANCH" "$WT_PATH" "$BASE"

        echo "[OK] Worktree creado: $WT_PATH"
        echo "[OK] Rama activa: $BRANCH (base: $BASE)"
        echo "[INFO] Trabaja dentro de ese directorio. Al verificar, ejecuta: scripts/axiom-worktree.sh pr $SLUG"
        ;;

    pr)
        if [ ! -d "$WT_PATH" ]; then
            echo "ERROR: No existe el worktree en: $WT_PATH" >&2
            exit 1
        fi

        DIRTY="$(git -C "$WT_PATH" status --porcelain)"
        if [ -n "$DIRTY" ]; then
            echo "ERROR: El worktree tiene cambios sin commitear. Haz commit de todo antes de crear el PR:" >&2
            echo "$DIRTY" >&2
            exit 1
        fi

        echo "[INFO] Subiendo $BRANCH a origin..."
        git -C "$WT_PATH" push -u origin "$BRANCH"

        REMOTE_URL="$(git -C "$WT_PATH" remote get-url origin | sed 's/\.git$//')"
        COMPARE_URL="$REMOTE_URL/compare/main...$BRANCH?expand=1"

        if command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
            echo "[INFO] Creando Pull Request mediante gh CLI..."
            (cd "$WT_PATH" && gh pr create --base main --fill)
            echo "[OK] Pull Request creado exitosamente."
        else
            echo "[AVISO] gh CLI no disponible o no autenticado. Abre el PR manualmente en GitHub:"
            echo "$COMPARE_URL"
        fi
        ;;

    done)
        CURRENT_BRANCH="$(git -C "$REPO_ROOT" rev-parse --abbrev-ref HEAD)"
        if [ "$CURRENT_BRANCH" = "$BRANCH" ]; then
            echo "ERROR: La rama $BRANCH está activa en el checkout principal. Cambia a main antes de eliminar." >&2
            exit 1
        fi

        if [ -d "$WT_PATH" ]; then
            echo "[INFO] Eliminando el worktree en $WT_PATH..."
            if [ "$FORCE" -eq 1 ]; then
                git -C "$REPO_ROOT" worktree remove --force "$WT_PATH"
            else
                git -C "$REPO_ROOT" worktree remove "$WT_PATH"
            fi
        else
            echo "[INFO] El directorio del worktree $WT_PATH no existe; continuando..."
        fi

        CURRENT_BRANCH="$(git -C "$REPO_ROOT" rev-parse --abbrev-ref HEAD)"
        if [ "$CURRENT_BRANCH" = "main" ]; then
            echo "[INFO] Actualizando main local con origin/main..."
            git -C "$REPO_ROOT" fetch origin main 2>/dev/null || true
            git -C "$REPO_ROOT" merge --ff-only origin/main 2>/dev/null || echo "[AVISO] No se pudo hacer ff-only en main."
        fi

        echo "[INFO] Eliminando rama local $BRANCH..."
        if [ "$FORCE" -eq 1 ]; then
            git -C "$REPO_ROOT" branch -D "$BRANCH" 2>/dev/null || true
        else
            git -C "$REPO_ROOT" branch -d "$BRANCH" 2>/dev/null || echo "[AVISO] No se pudo borrar $BRANCH con -d. Usa --force si ya está integrada."
        fi

        git -C "$REPO_ROOT" worktree prune
        echo "[OK] Ciclo de trabajo '$SLUG' desmantelado y entorno limpio."
        ;;
esac

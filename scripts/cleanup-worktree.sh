#!/usr/bin/env bash
# scripts/cleanup-worktree.sh — Desmantelamiento limpio y seguro de git worktrees y ramas locales post-merge (ODD-5.7).
set -euo pipefail

usage() {
  cat << 'EOF'
Uso: ./scripts/cleanup-worktree.sh <ruta-worktree> [nombre-rama] [--force]

Argumentos:
  <ruta-worktree>    Ruta al directorio del worktree a eliminar.
  [nombre-rama]      (Opcional) Nombre de la rama local asociada para eliminar tras desmontar el worktree.
  --force            Fuerza la eliminación incluso si hay cambios no commiteados o rama no mergeada.

Ejemplos:
  ./scripts/cleanup-worktree.sh ../wt-inc-01-auth feat/inc-01-auth
  ./scripts/cleanup-worktree.sh ../wt-inc-01-auth --force
EOF
  exit 1
}

if [ "$#" -lt 1 ]; then
  usage
fi

WORKTREE_PATH="$1"
shift

BRANCH_NAME=""
FORCE=false

while [ "$#" -gt 0 ]; do
  case "$1" in
    --force|-f)
      FORCE=true
      shift
      ;;
    *)
      if [ -z "$BRANCH_NAME" ]; then
        BRANCH_NAME="$1"
      fi
      shift
      ;;
  esac
done

if [ ! -d "$WORKTREE_PATH" ]; then
  echo "[AVISO] El directorio del worktree '$WORKTREE_PATH' no existe en disco. Ejecutando prune de referencias..."
  git worktree prune
  exit 0
fi

# Comprobar cambios sin commitear en el worktree
if [ -d "$WORKTREE_PATH/.git" ] || [ -f "$WORKTREE_PATH/.git" ]; then
  DIRTY_CHANGES=$(git -C "$WORKTREE_PATH" status --porcelain 2>/dev/null || true)
  if [ -n "$DIRTY_CHANGES" ] && [ "$FORCE" = false ]; then
    echo "[ERROR] El worktree en '$WORKTREE_PATH' contiene cambios sin commitear:"
    echo "$DIRTY_CHANGES"
    echo "Usa --force para descartar los cambios y forzar la eliminación."
    exit 1
  fi
fi

echo "Desmontando worktree en '$WORKTREE_PATH'..."
if [ "$FORCE" = true ]; then
  git worktree remove --force "$WORKTREE_PATH"
else
  git worktree remove "$WORKTREE_PATH"
fi

git worktree prune

if [ -n "$BRANCH_NAME" ]; then
  echo "Eliminando rama local '$BRANCH_NAME'..."
  if [ "$FORCE" = true ]; then
    git branch -D "$BRANCH_NAME" || true
  else
    git branch -d "$BRANCH_NAME" || {
      echo "[AVISO] No se pudo borrar la rama '$BRANCH_NAME' con -d (quizá no está mergeada en HEAD)."
      echo "Usa 'git branch -D $BRANCH_NAME' o vuelve a ejecutar con --force."
    }
  fi
fi

echo "[OK] Worktree '$WORKTREE_PATH' desmantelado exitosamente."

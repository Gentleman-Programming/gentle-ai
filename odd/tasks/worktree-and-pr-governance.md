# ODD: Metodología y Arnés de Worktrees y PRs en Axiom (Heredado de Ludeka)

> **Documento vivo ODD.** Fichero autoritativo: `odd/tasks/worktree-and-pr-governance.md`.  
> Espejo de recuperación en Engram: topic `odd/worktree-and-pr-governance/tasks`, proyecto `axiom`.

## Objetivo

Adoptar en el flujo de desarrollo de Axiom la metodología de aislamiento por worktree y ciclo de vida de Pull Requests probada en Ludeka, estandarizando las herramientas de arranque, gestión y desmantelamiento tanto en PowerShell como en POSIX Shell, y actualizando la gobernanza maestra del repositorio:

1. **Script de Ciclo de Vida PowerShell (`scripts/axiom-worktree.ps1`):**
   - Resolución canónica del repositorio raíz vía `--git-common-dir`.
   - Ubicación hermana aislada: `C:\repos\axiom-wt\<slug>` (o `../axiom-wt/<slug>`).
   - Verbos:
     - `new <slug>`: validación de slug kebab-case, comprobación de preexistencia, fetch de origin/main y creación del worktree + rama (`feat/<slug>` o `inc/<slug>`).
     - `pr <slug>`: validación de cambios sin commitear, push a origin con tracking y creación de Pull Request con `gh pr create --base main --fill` (o URL fallback a GitHub).
     - `done <slug>`: comprobación de checkout no activo, desmantelamiento del worktree, sincronización ff-only de main local, borrado de rama local y prune.
2. **Script de Ciclo de Vida POSIX Shell (`scripts/axiom-worktree.sh`):**
   - Equivalente funcional estricto para entornos Linux/macOS y runners Unix.
3. **Compatibilidad y Redirección Legada (`scripts/cleanup-worktree.ps1` & `.sh`):**
   - Mantener compatibilidad delegando o documentando el uso canónico de `axiom-worktree.ps1 done <slug>`.
4. **Reglas Maestras y Gobernanza (`AGENTS.md` y `GEMINI.md`):**
   - Prohibición estricta de commits o push directos a `main`.
   - Prohibición de uso de `EnterWorktree`/`ExitWorktree` volátiles de Claude Code.
   - Definición del mecanismo universal para todos los agentes (Antigravity, Gemini CLI, OpenCode, Claude Code).
   - Invocación estándar en Windows (`powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/axiom-worktree.ps1 ...`).
5. **Verificación Ejecutable:**
   - Comprobación de ciclo de vida completo de un worktree de prueba.

---

## Tareas

- [x] **T1 · Arnés PowerShell (`scripts/axiom-worktree.ps1`)**
  - Implementar script con resolución canónica de rutas, validaciones y verbos `new`, `pr`, `done`.
- [x] **T2 · Arnés POSIX Shell (`scripts/axiom-worktree.sh`)**
  - Implementar script equivalente en Bash con permisos de ejecución.
- [x] **T3 · Compatibilidad de scripts legados (`scripts/cleanup-worktree.ps1` y `.sh`)**
  - Adaptar o conectar con el nuevo arnés manteniendo compatibilidad.
- [x] **T4 · Actualización de Gobernanza (`AGENTS.md` y `GEMINI.md`)**
  - Incorporar la sección canónica de flujo de worktrees y PRs, eliminando ambigüedades.
- [x] **T5 · Verificación Funcional del Ciclo de Vida**
  - Probar `new`, validación de paths, y `done` limpiando rama y worktree de prueba.

---

## Verificación Ejecutable

- `powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts\axiom-worktree.ps1 new test-probe-wt` -> PASS (Worktree creado en `C:\repos\axiom-wt\test-probe-wt`, rama `feat/test-probe-wt`).
- `powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts\axiom-worktree.ps1 done test-probe-wt` -> PASS (Worktree eliminado, `main` actualizado, rama eliminada, prune ejecutado).
- `bash ./scripts/axiom-worktree.sh new test-bash-wt` -> PASS (Worktree creado en `/c/repos/axiom-wt/test-bash-wt`, rama `feat/test-bash-wt`).
- `bash ./scripts/axiom-worktree.sh done test-bash-wt` -> PASS (Worktree eliminado, rama eliminada, entorno limpio).
- `git worktree list` -> PASS (Sólo `C:/repos/axiom` en `main`).


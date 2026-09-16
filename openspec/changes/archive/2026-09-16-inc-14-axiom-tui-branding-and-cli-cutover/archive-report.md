# Reporte de Archivado: Unificación de TUI Bubbletea, Comandos de Ecosistema en CLI axiom y Pasarela de gentle-ai (INC-14)

**Fecha:** 2026-09-16  
**Incremento:** `inc-14-axiom-tui-branding-and-cli-cutover`  
**Estado:** ARCHIVED  

---

## Resumen del Incremento

El **Incremento 14 (INC-14: `axiom-tui-branding-and-cli-cutover`)** culmina la transición y soberanía del ecosistema unificando todos los comandos de terminal interactivo (TUI) y aprovisionamiento de herramientas bajo el ejecutable canónico `axiom`, al tiempo que dota a la TUI de una identidad visual sobria y moderna, y convierte el binario secundario `gentle-ai` en un wrapper de compatibilidad:

1. **Renovación Visual e Identidad de la TUI (`internal/tui/`):**
   - Sustitución en `internal/tui/styles/logo.go` de la silueta de la rosa de Gentle AI por un logotipo tipográfico en bloque ASCII de **AXIOM**, con renderizado de 5 bandas de gradiente en Lipgloss (Mauve, Lavender, Blue, Teal, Green).
   - Actualización de `Tagline(version)` en `internal/tui/styles/styles.go` al lema oficial de la plataforma: `"Axiom " + version + " — Plataforma SDD Multi-Rol y Multi-Repositorio"`.
   - Compatibilidad total de viewports y tests de interfaz responsiva en `internal/tui/screens`.

2. **Lanzamiento de TUI y Comandos de Ecosistema en `cmd/axiom/main.go`:**
   - Detección automática de terminal interactivo (TTY): al invocar `axiom` sin argumentos, si la sesión cuenta con TTY en `stdin` y `stdout`, se despliega directamente la TUI interactiva de Bubbletea.
   - En entornos no interactivos (CI, scripts), la invocación sin argumentos emite la ayuda textual `printHelp()` con código `0`.
   - Subcomando explícito `axiom tui` para lanzar la interfaz de terminal interactivamente bajo demanda.
   - Subcomandos de gestión de herramientas y agentes conectados directamente con `internal/app`:
     - `axiom install [flags]`
     - `axiom sync [flags]`
     - `axiom upgrade [flags]`
     - `axiom doctor [flags]`
     - `axiom backup` (con formateo visual y listado de snapshots de `~/.axiom/backups/`)
     - `axiom restore [flags]`
     - `axiom uninstall [flags]`
   - Actualización integral de la ayuda en `printHelp()`.

3. **Pasarela Ligera de Compatibilidad (`cmd/gentle-ai/main.go`):**
   - Transformación de `cmd/gentle-ai/main.go` en un wrapper de compatibilidad.
   - Emisión en `stderr` de la advertencia informativa: `Aviso: 'gentle-ai' está deprecado y ha sido unificado en 'axiom'. Se recomienda utilizar 'axiom' en su lugar.`.
   - Delegación completa y transparente hacia `internal/app.RunArgs(args, stdout)`.

4. **Pruebas y Validación Exhaustiva:**
   - Suite completa de pruebas en `cmd/axiom/main_test.go` y `cmd/gentle-ai/main_test.go` con 100% PASS.
   - Validación formal con `axiom sdd-verify-validate` (VERDICT: PASS, 4/4 requerimientos, 9/9 escenarios).

---

## Artefactos Consolidados y Modificados

- **Puntos de Entrada CLI:**
  - `cmd/axiom/main.go`
  - `cmd/axiom/main_test.go`
  - `cmd/gentle-ai/main.go`
  - `cmd/gentle-ai/main_test.go` (nuevo)
- **TUI y Estilos:**
  - `internal/tui/styles/logo.go`
  - `internal/tui/styles/styles.go`
- **Ciclo SDD de INC-14:**
  - `openspec/changes/inc-14-axiom-tui-branding-and-cli-cutover/proposal.md`
  - `openspec/changes/inc-14-axiom-tui-branding-and-cli-cutover/spec.md`
  - `openspec/changes/inc-14-axiom-tui-branding-and-cli-cutover/design.md`
  - `openspec/changes/inc-14-axiom-tui-branding-and-cli-cutover/tasks.md`
  - `openspec/changes/inc-14-axiom-tui-branding-and-cli-cutover/verify-report.md`
  - `openspec/changes/inc-14-axiom-tui-branding-and-cli-cutover/archive-report.md`
- **Especificaciones Vivas:**
  - `openspec/specs/axiom-tui-branding/spec.md` (nueva)
  - `openspec/INDEX.md` (sincronizado)

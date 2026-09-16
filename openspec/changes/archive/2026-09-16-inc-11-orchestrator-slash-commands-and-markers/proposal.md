# Propuesta: Renombrado de Orquestador a axiom-orchestrator, Comandos Slash Canónicos y Migración de Marcadores (INC-11)

## Propósito (Intent)

En la integración actual con editores y agentes de IA:
1. En **OpenCode**, el agente principal de coordinación se llama `gentle-orchestrator` en `opencode.json` y en las configuraciones multi-agente (`sdd-overlay-multi.json`). Los prompts de estado y despacho instruyen al modelo diciendo *"You are the gentle-orchestrator"*.
2. En **Claude Code**, todos los comandos slash correspondientes al ciclo SDD llevan el prefijo de la herramienta madre: `/gentle-sdd-new`, `/gentle-sdd-init`, `/gentle-sdd-apply`, `/gentle-sdd-verify`, etc. Esto resulta confuso y anacrónico para el flujo de trabajo de Axiom.
3. En la persistencia sobre el sistema de archivos, el sistema emplea marcadores HTML como `<!-- gentle-ai:sdd-orchestrator -->`, `<!-- gentle-ai:persona -->` y archivos centinela como `openspec/changes/<change>/.gentle-ai-instance`.

El **Incremento 11 (INC-11: `orchestrator-slash-commands-and-markers`)** renombra el orquestador oficial a **`axiom-orchestrator`**, normaliza los comandos slash a la convención estándar **`/sdd-*`** y migra los marcadores a la nomenclatura **`<!-- axiom:... -->`** y **`.axiom-instance`**, preservando total retrocompatibilidad con repositorios o configuraciones que contengan los marcadores antiguos.

---

## Alcance (Scope)

### Dentro de Alcance (In Scope)

1. **Renombrado del Orquestador a `axiom-orchestrator`:**
   - Actualizar las plantillas overlay de OpenCode (`internal/assets/opencode/sdd-overlay-multi.json` y `sdd-overlay-single.json`) sustituyendo la clave `gentle-orchestrator` por `axiom-orchestrator`.
   - Modificar `internal/opencode/config.go` para mapear el orquestador canónico como `axiom-orchestrator` (manteniendo `gentle-orchestrator` como alias legado de lectura).
   - Actualizar los comandos de OpenCode (`sdd-status.md`, `sdd-continue.md`, etc.) para declarar `agent: axiom-orchestrator` y referirse a sí mismo como el orquestador de Axiom.

2. **Estandarización de Comandos Slash en Claude Code:**
   - Renombrar los archivos en `internal/assets/claude/commands/`:
     - `gentle-sdd-new.md` ➔ `sdd-new.md`
     - `gentle-sdd-init.md` ➔ `sdd-init.md`
     - `gentle-sdd-explore.md` ➔ `sdd-explore.md`
     - `gentle-sdd-research.md` ➔ `sdd-research.md`
     - `gentle-sdd-apply.md` ➔ `sdd-apply.md`
     - `gentle-sdd-verify.md` ➔ `sdd-verify.md`
     - `gentle-sdd-archive.md` ➔ `sdd-archive.md`
     - `gentle-sdd-continue.md` ➔ `sdd-continue.md`
     - `gentle-sdd-status.md` ➔ `sdd-status.md`
     - `gentle-sdd-onboard.md` ➔ `sdd-onboard.md`
     - `gentle-sdd-ff.md` ➔ `sdd-ff.md`
   - El usuario podrá invocar directamente `/sdd-apply`, `/sdd-status`, `/sdd-verify`, etc., siguiendo el estándar natural de OpenSpec.

3. **Migración de Marcadores de Sección y Archivo de Instancia:**
   - Soportar marcadores canónicos:
     - `<!-- axiom:sdd-orchestrator --> ... <!-- /axiom:sdd-orchestrator -->`
     - `<!-- axiom:persona --> ... <!-- /axiom:persona -->`
     - `<!-- axiom:strict-tdd-mode --> ... <!-- /axiom:strict-tdd-mode -->`
   - Actualizar los inyectores y desinstaladores (`internal/components/uninstall/`) para reconocer tanto los marcadores nuevos `axiom:*` como los legados `gentle-ai:*`.
   - Renombrar el archivo marcador de instancia en cambios SDD a `.axiom-instance`, reconociendo `.gentle-ai-instance` si existiera en cambios creados anteriormente.

### Fuera de Alcance (Out of Scope)

- Modificación de la lógica interna de cálculo de dependencias de fases SDD.
- Migración del directorio de estado `~/.gentle-ai` (abordado en INC-12).

---

## Plan de Pruebas y Validación

- Pruebas unitarias en `internal/opencode/config_test.go` verificando la resolución de `axiom-orchestrator` y compatibilidad con `gentle-orchestrator`.
- Pruebas de instalación de comandos en Claude Code verificando los archivos `sdd-*.md`.
- Pruebas de inyección y limpieza de marcadores con `<!-- axiom:... -->` y `<!-- gentle-ai:... -->`.

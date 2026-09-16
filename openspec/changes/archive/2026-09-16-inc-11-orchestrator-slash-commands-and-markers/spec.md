# Especificación de Requerimientos: axiom-orchestrator y Comandos Slash Canónicos (INC-11)

## Propósito

Definir los requerimientos funcionales y escenarios de prueba para el agente orquestador `axiom-orchestrator`, la simplificación de comandos slash (`/sdd-*`) y el soporte dual de marcadores de sección.

---

## 1. Capacidad: `axiom-orchestrator-identity`

Identidad y configuración formal del agente orquestador en perfiles de OpenCode.

### Requirement: Registro de axiom-orchestrator en overlays de OpenCode (REQ-11.1)
El generador de configuración de OpenCode DEBE registrar el agente principal bajo la clave `axiom-orchestrator`, con descripción y prompt alineados con la metodología SDD de Axiom.

#### Scenario: Generación de opencode.json con agente axiom-orchestrator
- **DADO** una instalación o sincronización de agentes donde OpenCode está activo
- **CUANDO** se escribe el archivo `opencode.json`
- **ENTONCES** contiene la clave `"axiom-orchestrator"` en la sección `"agent"`
- **Y** la descripción indica "Axiom SDD Orchestrator - coordinates sub-agents, never does work inline"

### Requirement: Retrocompatibilidad de lectura con gentle-orchestrator (REQ-11.2)
Si un archivo `opencode.json` existente contiene asignaciones de modelo para `gentle-orchestrator`, el parser DEBE reconocerlas y aplicarlas a `axiom-orchestrator`.

#### Scenario: Migración transparente de configuraciones existentes
- **DADO** un `opencode.json` previo con `agent["gentle-orchestrator"].model = "openai/gpt-5.6-sol"`
- **CUANDO** Axiom lee la configuración del agente
- **ENTONCES** la asignación de modelo es adoptada por `axiom-orchestrator`
- **Y** no se pierden las elecciones de modelo previas del desarrollador

---

## 2. Capacidad: `canonical-sdd-commands`

Provisión de comandos slash limpios y estandarizados para Claude Code.

### Requirement: Comandos slash sin prefijo en Claude Code (REQ-11.3)
Los comandos slash instalados en `~/.claude/commands/` DEBEN utilizar la convención directa `/sdd-*` sin prefijos propietarios.

#### Scenario: Nombres de comandos slash en Claude Code
- **DADO** la instalación de comandos para Claude Code
- **CUANDO** se despliegan en el directorio de comandos del usuario
- **ENTONCES** los archivos creados son `sdd-apply.md`, `sdd-verify.md`, `sdd-archive.md`, `sdd-status.md`, `sdd-continue.md`, etc.
- **Y** no se crea ningún archivo con el prefijo `gentle-sdd-`

---

## 3. Capacidad: `dual-section-markers`

Soporte de marcadores de inyección en archivos Markdown y centinelas de cambio.

### Requirement: Soporte de marcadores axiom: con lectura de marcadores legados (REQ-11.4)
Las rutinas de inyección y eliminación de secciones en Markdown DEBEN emitir marcadores canónicos `<!-- axiom:<id> -->` y reconocer tanto `axiom:` como `gentle-ai:` para operaciones de limpieza o reemplazo.

#### Scenario: Inyección de secciones con marcador canónico
- **DADO** un archivo como `CLAUDE.md` o `SOUL.md` donde se inyecta el orquestador
- **CUANDO** se realiza la inyección de la sección de orquestación
- **ENTONCES** la sección queda delimitada por `<!-- axiom:sdd-orchestrator -->` y `<!-- /axiom:sdd-orchestrator -->`

#### Scenario: Limpieza de secciones con marcadores legados
- **DADO** un archivo que contiene delimitadores antiguos `<!-- gentle-ai:sdd-orchestrator -->`
- **CUANDO** se ejecuta la actualización o desinstalación
- **ENTONCES** la sección legada es detectada y purgada correctamente

# Propuesta: Integración de Comandos SDD en la CLI axiom y Actualización de Prompts (INC-13)

## Propósito (Intent)

Actualmente existe una dicotomía técnica en el repositorio:
1. **Dos binarios separados en `cmd/`:** El binario `axiom` (en `cmd/axiom/main.go`) ofrece comandos de gobierno de alto nivel (`axiom init`, `axiom project`, `axiom workspace`, `axiom handoff`, `axiom role`, `axiom skill`, `axiom semantic`, `axiom archive`, `axiom ui`), mientras que las operaciones centrales del ciclo Spec-Driven Development (`sdd-status`, `sdd-continue`, `sdd-attempt`, `review`, etc.) residen en el ejecutable `gentle-ai` (en `cmd/gentle-ai/main.go` / `internal/app/`).
2. **Prompts de agentes apuntando a la herramienta externa:** Todos los prompts del orquestador y subagentes (`sdd-orchestrator-sections.md`, `commands/sdd-status.md`, etc.) ordenan a los modelos de lenguaje ejecutar la CLI externa:
   - `gentle-ai sdd-status [change] ...`
   - `gentle-ai sdd-continue [change] ...`
   - `gentle-ai sdd-attempt acquire/settle ...`
   - `gentle-ai review start ...`
   Esto obliga a tener dos binarios instalados en la máquina del usuario (`axiom` y `gentle-ai`), generando duplicidad y confusión sobre cuál es la CLI autoritativa.

El **Incremento 13 (INC-13: `sdd-commands-axiom-cli-integration`)** integra de forma nativa los comandos del ciclo de vida SDD bajo la CLI unificada **`axiom`** (por ejemplo `axiom sdd status`, `axiom sdd continue`, `axiom sdd attempt`, o `axiom sdd-status` como alias) y actualiza todas las instrucciones de los prompts para que los agentes invoquen exclusivamente el comando **`axiom`**.

---

## Alcance (Scope)

### Dentro de Alcance (In Scope)

1. **Integración de Subcomandos SDD en `cmd/axiom/main.go`:**
   - Incorporar el grupo de comandos `axiom sdd`:
     - `axiom sdd status [change] [--json] [--instructions]`
     - `axiom sdd continue [change]`
     - `axiom sdd attempt <acquire|settle> ...`
     - `axiom sdd verify-validate ...`
     - `axiom sdd archive-compose ...`
   - Incorporar el grupo de comandos `axiom review`:
     - `axiom review start ...`
     - `axiom review capture-result ...`
     - `axiom review validate ...`
     - `axiom review status ...`
     - `axiom review mode <enable|disable|status> ...`
   - Proveer alias planos directos para compatibilidad con scripts existentes (`axiom sdd-status`, `axiom sdd-continue`, `axiom sdd-attempt`).
   - Conectar la ejecución con los paquetes de dominio existentes en `internal/sddstatus/`, `internal/attempt/`, `internal/reviewerprovider/`.

2. **Actualización de Prompts y Plantillas de Orquestación:**
   - Actualizar `internal/assets/skills/_shared/sdd-orchestrator-sections.md`:
     - Sustituir `gentle-ai sdd-status` por `axiom sdd status` (o `axiom sdd-status`).
     - Sustituir `gentle-ai sdd-continue` por `axiom sdd continue`.
     - Sustituir `gentle-ai sdd-attempt` por `axiom sdd attempt`.
     - Sustituir `gentle-ai review` por `axiom review`.
   - Actualizar los comandos slash de OpenCode y Claude Code (`internal/assets/opencode/commands/` y `internal/assets/claude/commands/`) para invocar el comando `axiom`.
   - Actualizar las skills de SDD (`internal/assets/skills/sdd-*/SKILL.md`).

3. **Unificación de la Experiencia de Usuario:**
   - La ayuda global `axiom help` documentará tanto los comandos de alto nivel (Hub, Multi-Rol, Living Doc, Dashboard) como los comandos de ejecución del ciclo SDD.
   - El desarrollador solo necesita compilar y utilizar un único binario: `axiom` (o `axiom.exe`).

### Fuera de Alcance (Out of Scope)

- Eliminación del código fuente de `cmd/gentle-ai/` en este incremento (se mantendrá temporalmente como pasarela de compatibilidad o wrapper ligero hasta completar el ciclo de transición).
- Modificación del backend de base de datos SQLite ni de la especificación OpenSpec.

---

## Plan de Pruebas y Validación

- Pruebas de integración CLI ejecutando `axiom sdd status` sobre un cambio activo y comprobando la salida JSON exacta.
- Pruebas unitarias de enrutamiento de comandos en `cmd/axiom/main_test.go`.
- Pruebas en `internal/assets/assets_test.go` verificando que los prompts ensamblados contienen `axiom sdd` en lugar de `gentle-ai`.

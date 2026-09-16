# Diseño Técnico: Integración de Comandos SDD en la CLI axiom y Actualización de Prompts (INC-13)

## Contexto y Motivación

Axiom gestiona el desarrollo dirigido por especificaciones (Spec-Driven Development, SDD) y la orquestación multi-agente en el repositorio. No obstante, existía una inconsistencia de integración:
1. **Doble Binario:** Las capacidades de gobierno de alto nivel (`init`, `project`, `workspace`, `handoff`, `role`, `skill`, `semantic`, `archive`, `ui`) se concentraban en `cmd/axiom/`, mientras que la ejecución del ciclo de vida SDD (`sdd-status`, `sdd-continue`, `sdd-attempt`, `sdd-verify-validate`, `sdd-archive-compose`, `review`) se despachaba a través del binario secundario `cmd/gentle-ai/`.
2. **Prompts Desalineados:** Los prompts base de orquestadores y comandos slash (Claude Code, OpenCode, Kimi, Qwen, Windsurf, etc.) instruían a los modelos de lenguaje a invocar la CLI externa `gentle-ai sdd-...`, forzando la presencia de dos binarios en el sistema del desarrollador.

El **Incremento 13 (INC-13: `inc-13-sdd-commands-axiom-cli-integration`)** resuelve esta fricción integrando directamente todas las operaciones del ciclo SDD bajo el comando unificado **`axiom`** (tanto en estructura jerárquica `axiom sdd <subcmd>` y `axiom review <subcmd>`, como mediante alias directos `axiom sdd-*` y `axiom review-*`), y actualizando los activos de prompts para que la plataforma opere de forma monolítica bajo su propia identidad.

---

## Decisiones de Arquitectura

### Decisión 1: Jerarquía y Enrutamiento Unificado en `cmd/axiom/main.go`

El punto de entrada `cmd/axiom/main.go` conectará de forma directa con los despachadores autoritativos ya existentes en `internal/cli`:

1. **Grupo de Comandos `axiom sdd`:**
   - `axiom sdd status [change] [flags]`: Invoca `cli.RunSDDStatus(args, os.Stdout)`.
   - `axiom sdd continue [change] [flags]`: Invoca `cli.RunSDDContinue(args, os.Stdout)`.
   - `axiom sdd attempt <acquire|settle> [flags]`: Invoca `cli.RunSDDAttempt(cli.CanonicalizeSDDAttemptRevisionArgs(args), os.Stdout)`.
   - `axiom sdd verify-validate [flags]`: Invoca `cli.RunSDDVerifyValidate(args, os.Stdout)`.
   - `axiom sdd archive-compose [flags]`: Invoca `cli.RunSDDArchiveCompose(args, os.Stdout)`.
   - `axiom sdd task-result [flags]`: Invoca `cli.RunSDDTaskResult(args, os.Stdout)`.
   - `axiom sdd preflight-hook [flags]`: Invoca `cli.RunSDDPreflightHook(args, os.Stdout)`.

2. **Grupo de Comandos `axiom review`:**
   - `axiom review mode <enable|disable|status> [flags]`: Invoca `cli.RunReviewMode(args, os.Stdout)`.
   - `axiom review start [flags]`: Invoca `cli.RunReviewStart(args, os.Stdout)`.
   - `axiom review resume [flags]`: Invoca `cli.RunReviewResume(args, os.Stdout)`.
   - `axiom review step [flags]`: Invoca `cli.RunReviewStep(args, os.Stdout)`.
   - `axiom review bundle-export [flags]`: Invoca `cli.RunReviewBundleExport(args, os.Stdout)`.
   - `axiom review bundle-import [flags]`: Invoca `cli.RunReviewBundleImport(args, os.Stdout)`.
   - `axiom review validate [flags]`: Invoca `cli.RunReviewValidateNonDeciding(args, os.Stdout)`.
   - `axiom review [flags]`: Invoca por defecto la fachada `cli.RunReview(args, os.Stdout)`.

3. **Alias Planos Directos:**
   Para preservar compatibilidad con scripts automatizados y agilidad de invocación directa:
   - `axiom sdd-status` -> reenvía a `sdd status`.
   - `axiom sdd-continue` -> reenvía a `sdd continue`.
   - `axiom sdd-attempt` -> reenvía a `sdd attempt`.
   - `axiom sdd-verify-validate` -> reenvía a `sdd verify-validate`.
   - `axiom sdd-archive-compose` -> reenvía a `sdd archive-compose`.
   - `axiom sdd-task-result` -> reenvía a `sdd task-result`.
   - `axiom sdd-preflight-hook` -> reenvía a `sdd preflight-hook`.
   - `axiom review-start`, `axiom review-resume`, `axiom review-step`, `axiom review-bundle-export`, `axiom review-bundle-import`, `axiom review-validate` -> reenvían al subcomando de revisión respectivo.

4. **Tratamiento de Códigos de Salida:**
   - Toda función `cli.Run*` devuelve un tipo `error`.
   - Si `err != nil`, se formatea a `os.Stderr` (`fmt.Fprintf(os.Stderr, "Error: %v\n", err)`) y se termina el proceso con `os.Exit(1)`.
   - Si la ejecución concluye con éxito (`err == nil`), el proceso finaliza con código `0`.

---

### Decisión 2: Actualización de la Ayuda Global (`printHelp`)

Se actualiza la salida de `axiom --help` / `axiom help` para incorporar la sección de desarrollo SDD:

```text
COMANDOS SDD Y REVISIÓN:
  sdd status           Consulta el estado de fases y artefactos de un cambio SDD (--json, --instructions)
  sdd continue         Calcula y emite la siguiente acción autorizada del despachador SDD
  sdd attempt          Gestiona el presupuesto y libro mayor de intentos de ejecución (acquire / settle)
  sdd verify-validate  Valida un reporte de verificación contra las especificaciones activas
  sdd archive-compose  Compone el reporte de archivado formal y actualiza las especificaciones vivas
  review               Gestiona el ciclo de vida de revisión formal RDD (start, resume, step, mode, validate)
```

---

### Decisión 3: Migración de Prompts de Orquestadores y Comandos Slash

Se actualizan todos los archivos de prompts y plantillas embebidas en `internal/assets/`:

1. **Secciones de Orquestación Compartidas (`internal/assets/skills/_shared/sdd-orchestrator-sections.md`):**
   - Reemplazar llamadas `gentle-ai sdd-status` por `axiom sdd status`.
   - Reemplazar llamadas `gentle-ai sdd-continue` por `axiom sdd continue`.
   - Reemplazar llamadas `gentle-ai sdd-attempt` por `axiom sdd attempt`.
   - Reemplazar llamadas `gentle-ai review` por `axiom review`.
2. **Flujos de Trabajo Específicos de Agentes:**
   - `claude/sdd-orchestrator-workflow.md`: prescribir `axiom sdd status`, `axiom sdd continue`, `axiom sdd attempt`.
   - `claude/commands/sdd-status.md`: invocar `axiom sdd status`.
   - `opencode/commands/sdd-status.md` y `opencode/commands/sdd-continue.md`: invocar `axiom sdd status` y `axiom sdd continue`.
   - `skills/_shared/sdd-status-contract.md`: referenciar `axiom sdd status` y `axiom sdd continue`.
   - `skills/_shared/sdd-phase-common.md`: referenciar `axiom sdd status`, `axiom sdd verify-validate`, `axiom sdd task-result`.
   - `skills/sdd-verify/SKILL.md` y `skills/sdd-archive/SKILL.md`: prescribir `axiom sdd verify-validate` y `axiom sdd archive-compose`.
3. **Compatibilidad de Pruebas Unitarias (`internal/assets/assets_test.go`):**
   - Adaptar los tests que verificaban cadenas exactas de `gentle-ai` para verificar los comandos canónicos de `axiom sdd` manteniendo, cuando aplique, soporte tolerante.

---

### Decisión 4: Estrategia de Pruebas Automatizadas

1. **Pruebas de Despacho de Comandos (`cmd/axiom/main_test.go`):**
   - Verificar que `axiom sdd status` ejecute y devuelva salida JSON esperada sobre un cambio real de prueba.
   - Verificar que `axiom sdd-status` (alias plano) produzca exactamente el mismo comportamiento.
   - Verificar que invocaciones inválidas o subcomandos inexistentes emitan código de error `1` y mensaje claro.
2. **Pruebas de Integridad de Prompts (`internal/assets/assets_test.go`):**
   - Asegurar que ningún prompt activo en `sdd-orchestrator-sections.md` ni en los comandos slash instruya a usar la CLI externa `gentle-ai`.

# Axiom — Reglas Maestras del Proyecto y Guía de Agentes

> **Proyecto:** Axiom (Fork y versión paralela de Gentle-AI)  
> **Propósito:** Evolución y desarrollo de flujos avanzados de IA, orquestación SDD/RDD y herramientas para agentes de codificación.  
> **Stack Técnico:** Go 1.25+ (Go 1.27.0 en entorno local), Bubbletea TUI (`charmbracelet/bubbletea`), Lipgloss, arquitectura de paquetes en `internal/`, arnés SDD nativo de Gentle-AI y memoria persistente Engram MCP.

---

## 0. REGLA SUPREMA: IDIOMA OBLIGATORIO — ESPAÑOL (CASTELLANO)

- **TODO EN ESPAÑOL:** Todas las respuestas del agente, mensajes de chat, explicaciones, resúmenes, razonamientos dirigidos al usuario y **TODOS los artefactos de SDD** (`proposal.md`, `spec.md`, `design.md`, `tasks.md`, `verify-report.md`, `archive-report.md`, `walkthrough.md`, etc.) DEBEN generarse y redactarse estrictamente en **español (castellano)**.
- **PROHIBIDO EL INGLÉS EN DOCUMENTACIÓN Y ARTEFACTOS:** Queda terminantemente prohibido generar propuestas, especificaciones, diseños o informes en inglés. La única excepción son los identificadores técnicos de código (nombres de paquetes, funciones, structs, interfaces y variables en Go) y palabras clave del lenguaje.
- **PRECEDENCIA:** Si cualquier skill, prompt o plantilla externa menciona "default to English", esta regla del proyecto TIENE PRECEDENCIA ABSOLUTA y la sobreescribe: genera SIEMPRE el contenido en español castellano.

---

## 1. Filosofía de Desarrollo: Flujo Dual ODD (Cotidiano) y SDD (Formal)

Axiom incorpora las mejores prácticas del flujo de trabajo moderno, ofreciendo dos vías complementarias según el alcance de la tarea:

### A. ODD (Organic Driven Development) — Vía Cotidiana y Ágil
Para el trabajo habitual, refactorizaciones y funcionalidades directas sin sobrecarga burocrática de fases:
1. **Documento Único de Tarea:** Se crea y mantiene un único archivo vivo `odd/tasks/<nombre-feature>.md` y su réplica sincronizada en Engram MCP bajo el tópico `odd/<nombre-feature>/tasks`.
2. **Ciclo Ágil:** `Exploración proporcional ➔ Implementación autorizada ➔ Comprobación funcional / TDD`.
3. **Control y Evidencia:** El archivo de tarea registra objetivos, restricciones, lista de tareas accionables con IDs estables, progreso y comandos de prueba ejecutados.
4. **Comandos canónicos ya operativos:** `axiom odd create <nombre>`, `axiom odd status [--json] [--check-mirror]` y `axiom odd promote <feature> [--dry-run] [--name <nombre>]`, con superficie reactiva en `axiom ui` (pestaña ODD) y `axiom tui` (Gobernanza → Carril Ágil ODD).

### B. SDD (Spec-Driven Development) — Vía Formal de Arquitectura
Reservado para grandes incrementos del sistema, cambios estructurales profundos o cuando el usuario pida explícitamente *"usa SDD"*:
```
[ sdd-explore ] ➔ [ sdd-propose ] ➔ [ sdd-spec ] ➔ [ sdd-design ] ➔ [ sdd-tasks ] ➔ [ sdd-apply ] ➔ [ sdd-verify ] ➔ [ sdd-archive ]
```
1. **File-System como Fuente de la Verdad:** El estado de las fases reside en `openspec/` y `.openspec/`.
2. **Lossless Blocking Prompts:** Antes de pasar de propuesta a especificación o de diseño a implementación, presentar las decisiones al usuario en español y esperar confirmación.
3. **Determinación temprana de carril y cuestionario de pre-vuelo (INC-21):** antes de crear `proposal.md`, sellar la modalidad de avance (continua o con paradas), la política de relevos y el roster de roles con `axiom sdd kickoff seal`; sin subdivisión de roles se asigna obligatoriamente el rol único `fullstack`. En modalidad con paradas, cuatro compuertas de bloque (`spec`, `design`, `tasks`, `apply` por rol) exigen una decisión explícita con `axiom sdd gate record --gate <clave> --decision approved|rejected --reason "<motivo>"` antes de avanzar a la siguiente fase o rol; un rechazo exige remediación y vuelve a presentar la misma compuerta. Esta gobernanza es distinta de RDD: una compuerta aprobada nunca es un recibo ni autoriza la entrega por sí misma.
4. **Comandos Canónicos de Axiom:** Utilizar los comandos integrados en la CLI `axiom`:
   - `axiom sdd kickoff seal|show [cambio]`
   - `axiom sdd gate record|show [cambio]`
   - `axiom sdd status [cambio]`
   - `axiom sdd continue [cambio]`
   - `axiom sdd archive-compose [cambio]`
   - O bien gestionar el ciclo interactivamente desde el Dashboard Web local: `axiom ui`.
5. **Verificación Formal Obligatoria:** A diferencia de upstream, en Axiom la verificación con tests automatizados y reporte formal (`verify-report.md`) es un pilar innegociable antes del archivado.
6. **Precondición de integración o despliegue para `archive` (INC-21):** `archive` no procede como paso inmediato tras `verify-report.md` favorable; exige evidencia de integración o despliegue registrada con `axiom sdd gate record --gate integration --decision approved --evidence-kind pr_merged|deployment|attestation [--commit <sha>]`. Un incremento ya archivado queda congelado: cualquier corrección posterior se gestiona mediante un ticket de bug o un nuevo incremento, nunca modificando directamente `openspec/changes/archive/`.

### C. Política Git Diferencial, Sincronización Continua y Worktrees (ODD-5)
1. **Repositorio de Especificaciones (Directo a Main):** El repositorio `specs_repository` opera SIEMPRE en la rama principal (`main`/`master`), sin ramas ni worktrees. Al emitir relevos inter-fase (`handoff.md`), es obligatorio confirmar commit y push de las especificaciones y diseños.
2. **Sincronización al Inicio de Sesión del Agente:** Al iniciar una sesión de trabajo, el agente ejecuta pasivamente `git fetch` en el repositorio de specs. Si detecta commits remotos entrantes (`behind > 0`), avisa de inmediato al usuario y solicita autorización para ejecutar `git pull` antes de operar.
3. **Repositorios de Código (Worktrees y Ramas Aisladas):** En repositorios de código fuente, los roles aplican un pre-vuelo interactivo: (1) worktree aislado vs árbol actual, (2) nombre de rama (`feat/...`), (3) rama base (`main`, `develop`, etc.).
4. **Cierre de Rol con Selección de Rama Destino en PR:** Al validar el informe del rol en PASS, se ofrece registrar el commit y se consulta la rama destino del Pull Request (`develop`, `main`, etc.) antes de proponer `gh pr create --base <rama-destino>`.
5. **Limpieza Post-Merge:** Uso de los scripts auxiliares (`scripts/cleanup-worktree.sh` y `.ps1`) para eliminar limpiamente el worktree y la rama local tras el merge.

---

## 2. Protocolo de Memoria Persistente (Engram MCP)

El proyecto cuenta con el servidor MCP de **Engram** conectado bajo el espacio de proyecto `axiom`.
- **Cuándo guardar (`mem_save`):** Inmediatamente tras resolver un bug, tomar una decisión de diseño, aprender una regla interna de Go/Axiom o establecer un patrón.
- **Cuándo consultar (`mem_context` / `mem_search`):** Al inicio de sesión o al retomar una funcionalidad para no perder el contexto previo.
- **Cierre de sesión (`mem_session_summary`):** Obligatorio antes de finalizar la sesión de trabajo.

---

## 3. Estándares Técnicos (Go & Arquitectura)

- **Estructura del Proyecto:**
  - `cmd/`: Puntos de entrada ejecutables (CLI de la herramienta).
  - `internal/`: Paquetes y lógica interna del sistema (agentes, TUI, modelo, pipeline, catálogo, verificadores, etc.).
  - `openspec/`: Especificaciones vivas, cambios en curso (`openspec/changes/`) y cambios archivados (`openspec/changes/archive/`).
  - `skills/`: Skills de desarrollo y flujos de trabajo de Gentle-AI.
- **Pruebas y Calidad:**
  - `go test ./...` para pruebas unitarias.
  - `go vet ./...` para validación estática.
  - `gofmt` para formateo estándar de Go.
- **Repositorio de Referencia Upstream (v3):**
  - Código fuente local de la última versión de Gentle-AI (v3.0.2+): `c:/repos/gentle-ai` disponible para consultas de código, diffs y análisis de Git.

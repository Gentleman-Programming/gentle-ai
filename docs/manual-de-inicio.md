# Manual de Arranque y Operación de Axiom

> **Axiom** — Plataforma Determinista de Ingeniería de Software, Orquestación Multi-Agente y Spec-Driven Development (SDD).  
> **Destinatarios:** Equipos de ingeniería, arquitectos de software y desarrolladores que se incorporan al ecosistema.

---

## Índice

1. [Filosofía y Arquitectura](#1-filosofía-y-arquitectura)
2. [Prerrequisitos del Sistema](#2-prerrequisitos-del-sistema)
3. [Instalación en un Equipo Nuevo](#3-instalación-en-un-equipo-nuevo)
4. [Diagnóstico Inicial de Salud (`doctor`)](#4-diagnóstico-inicial-de-salud-doctor)
5. [Inicialización y Configuración de Proyectos](#5-inicialización-y-configuración-de-proyectos)
   - [Topologías Soportadas](#topologías-soportadas)
   - [Opción A: Línea de Comandos (CLI)](#opción-a-línea-de-comandos-cli)
   - [Opción B: Panel Web Interactivo (`axiom ui`)](#opción-b-panel-web-interactivo-axiom-ui)
   - [Aprovisionamiento Aislado de Agentes (`setup`)](#aprovisionamiento-aislado-de-agentes-setup)
6. [El Archivo de Configuración Maestro (`axiom.yaml`)](#6-el-archivo-de-configuración-maestro-axiomyaml)
7. [Gobernanza de Skills y Autoskills](#7-gobernanza-de-skills-y-autoskills)
   - [Flujo Human-in-the-Loop](#flujo-human-in-the-loop)
   - [Uso por CLI y por Web UI](#uso-por-cli-y-por-web-ui)
8. [Gestión de Especificaciones Vivas (OpenSpec)](#8-gestión-de-especificaciones-vivas-openspec)
9. [Flujo Dual de Desarrollo: ODD Cotidiano vs. SDD Formal](#9-flujo-dual-de-desarrollo-odd-cotidiano-vs-sdd-formal)
10. [Relevos Estructurados entre Roles y Fases (Handoffs)](#10-relevos-estructurados-entre-roles-y-fases-handoffs)
11. [Mantenimiento, Actualizaciones y Sincronización](#11-mantenimiento-actualizaciones-y-sincronización)

---

## 1. Filosofía y Arquitectura

Los agentes de inteligencia artificial para desarrollo de software sufren frecuentemente de dos limitaciones críticas:
1. **Pérdida de contexto:** Cada sesión arranca en blanco o el contexto se degrada tras compactaciones.
2. **Falta de determinismo:** Los modelos no deben "votar" qué fase sigue en un proyecto ni inventar rutas críticas.

**Axiom** resuelve estos problemas estructurando el desarrollo mediante:
* **Memoria Persistente (Engram):** Contexto acumulativo entre sesiones y agentes.
* **Gobernanza Determinista:** El estado del proyecto reside en el sistema de archivos (`axiom.yaml`, `openspec/`) y una CLI estricta en Go determina la única transición válida en cada momento.
* **Aislamiento por Workspace:** Axiom puede auto-programarse y gestionar repositorios sin sobreescribir configuraciones globales de usuario ni colisionar con otros proyectos.
* **Paridad de Interfaces:** Todo lo ejecutable por línea de comandos tiene su reflejo interactivo en la TUI de terminal (`axiom tui`) y en el Dashboard Web (`axiom ui`).

---

## 2. Prerrequisitos del Sistema

Antes de instalar Axiom en una máquina nueva, asegúrate de contar con:

* **Go 1.25+** (Axiom está compilado y validado en Go 1.25+):  
  Verifica con: `go version`
* **Git 2.38+**:  
  Verifica con: `git --version`
* **Node.js (18+) y npm** *(necesario para herramientas satélite y análisis semántico)*:  
  Verifica con: `node -v` y `npm -v`
* **Tus agentes de IA habituales** instalados en el sistema (por ejemplo OpenCode, Claude Code, Pi, Cursor, etc.).

---

## 3. Instalación en un Equipo Nuevo

Axiom se compila e instala en el `PATH` del sistema mediante la herramienta estándar de Go:

### Canal de Desarrollo Continuo (`main` — Recomendado para novedades diarias)
Permite obtener inmediatamente las últimas mejoras mergeadas sin esperar a una release empaquetada:
```bash
# Windows (PowerShell) / Linux / macOS
go install github.com/IGutierrezZ/axiom/cmd/axiom@main
```

### Canal Estable (`latest` — Hitos consolidados)
```bash
go install github.com/IGutierrezZ/axiom/cmd/axiom@latest
```

### Compilación desde el Código Fuente Local
Si has clonado el repositorio de Axiom:
```bash
cd /ruta/hacia/axiom
go install ./cmd/axiom
```

El ejecutable `axiom` (o `axiom.exe` en Windows) quedará disponible inmediatamente en tu terminal dentro de `$GOPATH/bin` (habitualmente `%USERPROFILE%\go\bin` en Windows o `~/go/bin` en Linux/macOS).

---

## 4. Diagnóstico Inicial de Salud (`doctor`)

Tras instalar, ejecuta las comprobaciones integradas:

```bash
# 1. Comprobar la versión instalada
axiom version

# 2. Diagnóstico exhaustivo del ecosistema
axiom doctor
```

`axiom doctor` audita:
* Binarios requeridos en el `PATH` (Git, Go, Node.js, npm, etc.).
* Agentes de IA detectados en el sistema.
* Disponibilidad de servidores MCP y memoria Engram.
* Integridad de herramientas semánticas (Serena, CodeGraph).

---

## 5. Inicialización y Configuración de Proyectos

Axiom puede gobernar cualquier proyecto nuevo o existente en tres topologías:

### Topologías Soportadas

1. **`monorepo-embedded` (Por defecto):**  
   El código del proyecto y las especificaciones residen en el mismo repositorio Git (bajo la carpeta `openspec/`). Ideal para la mayoría de aplicaciones, microservicios agrupados o proyectos como el propio Axiom.
2. **`monorepo-decoupled`:**  
   El código está en un único repositorio, pero las especificaciones residen en un repositorio Git independiente clonado dentro de la misma carpeta raíz de trabajo.
3. **`multirepo`:**  
   Múltiples repositorios de código independientes (por ejemplo `frontend/`, `backend/`, `infra/`), coordinados bajo una carpeta paraguas común, con un repositorio canónico obligatorio para las especificaciones (`specs_repository`).

---

### Opción A: Línea de Comandos (CLI)

#### 1. Para un Monorepo
Sitúate en la raíz del repositorio y ejecuta:
```bash
axiom init --name "MiProyecto"
```
El motor de detección heurística de Axiom identificará automáticamente el lenguaje primario, los frameworks (Go, Node, React, Python, etc.) y generará:
* El contrato `axiom.yaml`.
* Las carpetas canónicas: `openspec/specs/`, `openspec/changes/` y `.axiom/inbox/skills/`.
* El registro del proyecto en el Hub global de workspaces (`~/.axiom/workspaces.json`).

#### 2. Para un Multirepo
Sitúate en la **carpeta contenedora maestra** (la carpeta que contiene los subdirectorios de cada repositorio de código y el repositorio de specs) y ejecuta:
```bash
axiom init --name "MiProyecto" --topology multirepo
```

---

### Opción B: Panel Web Interactivo (`axiom ui`)

Si prefieres una experiencia visual:

1. Lanza el dashboard desde cualquier terminal:
   ```bash
   axiom ui
   ```
2. Axiom abrirá automáticamente tu navegador en el servidor local (ej. `http://localhost:4040`).
3. En la barra superior o sección **Workspace / Proyectos**:
   * Pulsa en **"Agregar Proyecto"** o **"Inicializar Proyecto con Axiom (1 Clic)"**.
   * Introduce la ruta física (`path`), el nombre del proyecto y selecciona la topología (`monorepo-embedded`, `monorepo-decoupled` o `multirepo`).
   * El dashboard ejecutará en segundo plano el detector de tecnologías y creará la estructura sin necesidad de escribir comandos.
   * Puedes conmutar de un proyecto a otro instantáneamente desde el selector desplegable.

---

### Aprovisionamiento Aislado de Agentes (`setup`)

Una vez inicializado el proyecto, ejecuta en la raíz del mismo:

```bash
axiom setup
```

**¿Qué hace `axiom setup`?**
* Ejecuta la instalación con **ámbito de workspace (`--scope=workspace`)**.
* Crea o actualiza la configuración local del proyecto (por ejemplo `opencode.json` con `"default_agent": "axiom-orchestrator"`).
* **Garantía de Aislamiento:** No altera ni sobreescribe los archivos globales de usuario en `~/.config/opencode/` ni `~/.claude/`. Cada proyecto mantiene sus agentes, plugins y prompts de forma estrictamente local.

---

## 6. El Archivo de Configuración Maestro (`axiom.yaml`)

El archivo `axiom.yaml` es la única fuente de verdad técnica de la estructura del proyecto. Ejemplo canónico:

```yaml
workspace:
  name: "MiProyecto"
  topology: "monorepo-embedded"   # Opciones: monorepo-embedded | monorepo-decoupled | multirepo
  specs_repository: "openspec"    # Carpeta local o subrepositorio Git de especificaciones
  root: "."

roles:
  core:
    name: "Backend Core Service"
    repositories:
      - path: "."
    tech:
      - "go"
      - "postgres"
    gate_policy: "blocking"       # 'blocking' detiene el avance hasta aprobar; 'advisory' es informativo

  frontend:
    name: "Web Application"
    repositories:
      - path: "frontend"
    tech:
      - "typescript"
      - "react"
    gate_policy: "blocking"

governance:
  language: "es"                  # Idioma obligatorio de comunicación y artefactos
  shared_memory: "engram"         # Conector de memoria persistente
  semantic_analysis: "enabled"    # Serena y CodeGraph
```

Para validar en cualquier momento que la estructura de repositorios y roles cumple con las reglas:
```bash
axiom workspace validate
```

---

## 7. Gobernanza de Skills y Autoskills

Axiom integra un ecosistema de habilidades (*skills*) basado en el registro oficial auditado de `midudev/autoskills` (verificado criptográficamente con SHA-256) sumado a un motor de minería heurística de código local.

### Flujo Human-in-the-Loop

Axiom nunca instala una habilidad de IA a ciegas en tu proyecto. Sigue un flujo riguroso:

```
[ Escaneo / Minería ] ➔ [ Buzón Transitorio ] ➔ [ Aprobación Humana ] ➔ [ Promoción e Indexación ]
```

### Uso por CLI

1. **Escanear tecnologías y minar patrones:**
   ```bash
   axiom skill scan
   ```
   *(Las skills candidatas se depositan en el buzón transitorio `.axiom/inbox/skills/`).*

2. **Listar propuestas en el buzón:**
   ```bash
   axiom skill list --inbox
   ```

3. **Aprobar una skill para el proyecto:**
   ```bash
   axiom skill approve <nombre-de-la-skill>
   ```
   Al aprobarla, se copia formalmente a `skills/<nombre>/SKILL.md` y se regenera automáticamente el índice.

4. **Descartar una propuesta:**
   ```bash
   axiom skill reject <nombre-de-la-skill>
   ```

5. **Sincronizar el catálogo unificado en todos los destinos:**
   ```bash
   axiom skill index refresh
   ```
   Esto actualiza simultáneamente:
   * `.atl/skill-registry.md`
   * La tabla de habilidades en `AGENTS.md`
   * El tópico correspondiente en la memoria persistente de Engram MCP.

### Uso por Web UI (`axiom ui`)

1. Ve a la pestaña **Skills / Autoskills** en el panel web.
2. Pulsa en **"Escanear Skills"**.
3. Revisa la lista de habilidades propuestas, con su justificación, tecnologías asociadas y contenido.
4. Pulsa **"Aprobar"** o **"Rechazar"** directamente con un solo clic.

---

## 8. Gestión de Especificaciones Vivas (OpenSpec)

Axiom adopta y potencia el estándar **OpenSpec**:

* `openspec/specs/<dominio>/spec.md`: Especificaciones vivas consolidadas del sistema.
* `openspec/changes/<incremento>/`: Ciclos de cambio activos o en fase de propuesta.
* `openspec/changes/archive/`: Registro histórico inmutable de incrementos completados.
* `openspec/INDEX.md`: Catálogo maestro indexado de requerimientos y capacidades.

### Compatibilidad y Migración
* **Compatibilidad Directa:** Si un proyecto ya dispone de especificaciones en formato Markdown / OpenSpec, Axiom y su Dashboard Web las leen de forma nativa e inmediata.
* **Síntesis desde Cambios Previos (`coldstart`):** Si dispones de carpetas de cambios previos y necesitas reconstruir el catálogo de especificaciones vivas:
  ```bash
  axiom archive coldstart <nombre-cambio>
  axiom archive sync
  ```
* **Visor Web:** En `axiom ui`, la pestaña **Especificaciones** ofrece un árbol de navegación, buscador y visor con renderizado Markdown en tiempo real.

---

## 9. Flujo Dual de Desarrollo: ODD Cotidiano vs. SDD Formal

Axiom respeta la naturaleza del trabajo de ingeniería ofreciendo dos carriles complementarios:

### A. Carril Cotidiano: ODD (Organic-Driven Development)
Para el 90% del trabajo diario: tareas concretas, correcciones de errores y refactorizaciones sin burocracia de fases:
* **Comando:** Se gestiona mediante `axiom odd create <nombre>` o creando el documento vivo `odd/tasks/<nombre>.md`.
* **Seguimiento:** `axiom odd status` o la pestaña **ODD** en `axiom ui`.
* **Promoción:** Si una tarea ODD crece y requiere arquitectura formal, se eleva a SDD con:
  ```bash
  axiom odd promote <nombre>
  ```

### B. Carril Formal: SDD (Spec-Driven Development)
Para cambios estructurales profundos, refactorizaciones de arquitectura o requerimientos críticos:
```
[ sdd-explore ] ➔ [ sdd-propose ] ➔ [ sdd-spec ] ➔ [ sdd-design ] ➔ [ sdd-tasks ] ➔ [ sdd-apply ] ➔ [ sdd-verify ] ➔ [ sdd-archive ]
```
* **Inicio y Sello:** `axiom sdd kickoff seal` fija los roles y la modalidad de avance.
* **Compuertas de Bloque:** Cada fase requiere aprobación formal antes de avanzar (`axiom sdd gate record`).
* **Verificación y Archivado:** Exige informe de verificación con tests automáticos (`verify-report.md`) y evidencia de integración antes de congelar el cambio en `openspec/changes/archive/`.

---

## 10. Relevos Estructurados entre Roles y Fases (Handoffs)

Cuando un proyecto cuenta con múltiples roles (por ejemplo `core`, `frontend`, `qa`) o transita entre fases de arquitectura e implementación, el traspaso de contexto no puede depender de mensajes informales en un chat. Axiom formaliza este paso mediante el documento canónico **`handoff.md`**.

### Estructura Canónica del Relevo (`handoff.md`)
Cada relevo se redacta en castellano peninsular y se divide en 5 secciones obligatorias y un Frontmatter YAML con metadatos:

```markdown
---
change: "inc-05-mi-funcionalidad"
from_phase: "design"
to_phase: "tasks"
from_role: "core"
to_role: "qa"
timestamp: 2026-09-23T20:00:00Z
status: "ready" # 'ready', 'blocked' o 'needs_clarification'
---

## 1. Resumen Ejecutivo
Síntesis clara de los objetivos completados y el estado actual del incremento.

## 2. Artefactos Modificados y Creados
Rutas exactas a los archivos de especificación, diseño, código fuente o pruebas generadas.

## 3. Decisiones Técnicas y Acuerdos
Compromisos de arquitectura adoptados, patrones empleados y dependencias establecidas.

## 4. Riesgos, Bloqueos y Preguntas Abiertas
Puntos de fricción, dependencias pendientes de terceros o dudas que requieren respuesta.

## 5. Instrucciones Directas para el Siguiente Rol
Órdenes claras, secuenciales y accionables para el agente o ingeniero que toma el relevo.
```

### Comandos de Relevo (CLI)

1. **Crear una plantilla de relevo validada:**
   ```bash
   axiom handoff create --change <slug-cambio> --from <rol-emisor> --to <rol-receptor> --from-phase <fase-origen> --to-phase <fase-destino>
   ```
2. **Consultar el relevo activo de un cambio:**
   ```bash
   axiom handoff show --change <slug-cambio>
   ```
3. **Validar la consistencia y reglas de transición:**
   ```bash
   axiom handoff validate --change <slug-cambio>
   ```

### Réplica en Memoria Persistente (Engram MCP) y Web UI
* **Espejo Automático:** Al crearse o actualizarse un relevo, Axiom lo replica en Engram bajo el tópico `sdd/<slug-cambio>/handoff`. El nuevo agente que asuma el trabajo consulta su memoria antes de re-leer ficheros innecesarios.
* **Superficie Visual:** En `axiom ui`, la pestaña **Relevos / Handoffs** permite inspeccionar gráficamente la cadena de relevos, los roles involucrados y su estado de bloqueo.

---

## 11. Mantenimiento, Actualizaciones y Sincronización

### Actualizar Axiom en tu Máquina

1. **Desde la rama `main` (desarrollo activo continuo):**
   ```bash
   go install github.com/IGutierrezZ/axiom/cmd/axiom@main
   ```
2. **Mediante el comando `upgrade`:**
   ```bash
   # Utilizando el canal beta/nightly (apuntando a main):
   axiom upgrade --channel beta

   # O canal estable:
   axiom upgrade
   ```

### Sincronizar Agentes en el Repositorio Local (Workspace-Scoped)
En Axiom, la sincronización por defecto debe acotarse al ámbito del proyecto para aislar configuraciones y evitar colisiones:

```bash
axiom sync --scope=workspace
```

Este comando asegura que los prompts, agentes locales (`opencode.json`) y herramientas del proyecto reflejen las últimas directivas sin tocar tu configuración global de usuario en `~/.config/opencode/`.

### Automatización Post-Archivado y Scripts de Soporte

Para acelerar el flujo tras finalizar un incremento o generar una release:

1. **Actualización local tras archivar un incremento (`scripts/post-archive-sync.ps1`):**  
   Tras mergear y archivar un cambio en `main`, este script alinea la rama `main` local, recompila el binario `axiom` con `go install ./cmd/axiom` y ejecuta `axiom sync --scope=workspace` en tu máquina para dejar el entorno 100% operativo al instante.
2. **Lanzamiento de Release Oficial (`scripts/release-milestone.ps1`):**  
   Cuando un incremento represente un hito mayor consolidado, se ejecuta pasando el número de versión (ej. `./scripts/release-milestone.ps1 -Version v0.2.0`). El script valida que el repositorio esté limpio, genera el tag anotado en Git y hace `push` hacia GitHub, disparando el flujo automatizado de GoReleaser sin intervención manual.

---

*Axiom — Rigor de Arquitectura, Determinismo y Memoria para Agentes de Software.*

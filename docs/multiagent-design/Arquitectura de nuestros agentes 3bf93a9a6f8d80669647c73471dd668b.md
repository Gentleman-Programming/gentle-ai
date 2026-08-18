# Arquitectura de nuestros agentes

Sí. Con todo lo que ya definimos, **el diagrama anterior ya se nos quedó corto**. Sirve para explicar el SDD básico, pero ahora tenemos que mostrar también:

- cómo entra un requerimiento;
- cómo se identifica y traza;
- cómo el Orchestrator decide qué repositorios/agents/skills necesita;
- qué ocurre si es un sistema existente o un proyecto nuevo;
- cómo interviene base de datos;
- cómo se reparten las Tasks;
- cómo terminamos conectando Git/MR/Build/Release;
- y, sobre todo, distinguir **lo que ya tenemos definido/configurado de lo que aún falta implementar e integrar**.

La base que ya tienen sigue siendo válida: SDD separa exploración, propuesta, especificación, diseño, tareas, implementación y verificación en responsabilidades distintas.  Y el diseño actual de Skills ya plantea que el delegador resuelva reglas y se las inyecte a cada subagente, en vez de cargarle todo el Registry.

# Flujo objetivo actualizado del sistema multiagente

Este sería el diagrama que usaremos.

```latex
flowchart TD

    %% =========================================================
    %% 0. ENTRADA / TRAZABILIDAD
    %% =========================================================

    subgraph P0["0. ENTRADA / TRAZABILIDAD"]
        TDR["TDR-007<br/>(opcional)"]
        REQ["REQ-042"]
        ORCH["DEV ORCHESTRATOR"]

        TRACE["Trace Resolver"]
        REPORES["Repository Resolver"]
        SKILLRES["Skill Resolver"]
        CONTEXT["Context Builder"]

        TDR -. "Originates-From" .-> REQ
        REQ --> ORCH

        ORCH --> TRACE
        ORCH --> REPORES
        ORCH --> SKILLRES

        TRACE --> CONTEXT
        REPORES --> CONTEXT
        SKILLRES --> CONTEXT
    end

    %% =========================================================
    %% 1. DISCOVERY / CLASIFICACIÓN
    %% =========================================================

    subgraph P1["1. DISCOVERY / CLASIFICACIÓN DEL CAMBIO"]

        IMPACT["Impact Discovery"]

        TYPE{"¿Qué tipo de cambio?"}

        EXIST["EXISTING<br/>Sistema existente"]
        GREEN["GREENFIELD<br/>Proyecto nuevo"]
        MIXED["MIXED<br/>Existente + nuevo"]

        REG_EXIST["Repository Registry"]
        REG_GREEN["Repository Registry"]
        REG_MIX["Repository Registry"]

        ARCH_GREEN["Architecture Catalog"]
        ARCH_MIX["Architecture Catalog"]

        EXP_EXIST["Explorers<br/>por repositorio"]
        EXP_MIX["Explorers<br/>por repositorio"]

        SOL_GREEN["Solution Architect"]
        SOL_MIX["Solution Architect"]

        BLUE_GREEN["BLUEPRINT-012<br/>Project Blueprint"]
        BLUE_MIX["BLUEPRINT-013<br/>Project Blueprint"]

        EXP["EXP-042<br/>Exploration Artifact"]

        CONTEXT --> IMPACT
        IMPACT --> TYPE

        TYPE --> EXIST
        TYPE --> GREEN
        TYPE --> MIXED

        %% Existing
        EXIST --> REG_EXIST
        REG_EXIST --> EXP_EXIST
        EXP_EXIST --> EXP

        %% Greenfield
        GREEN --> REG_GREEN
        GREEN --> ARCH_GREEN

        REG_GREEN --> SOL_GREEN
        ARCH_GREEN --> SOL_GREEN

        SOL_GREEN --> BLUE_GREEN
        BLUE_GREEN --> EXP

        %% Mixed
        MIXED --> REG_MIX
        MIXED --> ARCH_MIX

        REG_MIX --> EXP_MIX
        REG_MIX --> SOL_MIX
        ARCH_MIX --> SOL_MIX

        SOL_MIX --> BLUE_MIX

        EXP_MIX --> EXP
        BLUE_MIX --> EXP
    end

    %% =========================================================
    %% 2. PROPUESTA / ARQUITECTURA / IMPACTO
    %% =========================================================

    subgraph P2["2. PROPUESTA / ARQUITECTURA / IMPACTO"]

        PROPOSER["dev-proposer"]
        PROP["PROP-018"]

        HG1{"HUMAN GATE #1<br/>Scope / Approach"}

        SPECIFIER["dev-specifier"]
        DESIGNER["dev-designer"]

        SPEC1["SPEC-061"]
        SPEC2["SPEC-062"]

        DESIGN["DESIGN-018"]

        DBIMPACT{"¿Impacto en Base de Datos?"}

        DBNONE["Sin revisión DB especializada"]
        DBSPEC["database-specialist"]
        DBASSESS["DBASSESS-021"]

        HG2{"HUMAN GATE #2<br/>Technical Gate"}

        EXP --> PROPOSER
        PROPOSER --> PROP
        PROP --> HG1

        HG1 -->|Approved| SPECIFIER
        HG1 -->|Approved| DESIGNER
        HG1 -->|Rework| PROPOSER

        SPECIFIER --> SPEC1
        SPECIFIER --> SPEC2

        DESIGNER --> DESIGN

        SPEC1 --> DBIMPACT
        SPEC2 --> DBIMPACT
        DESIGN --> DBIMPACT

        DBIMPACT -->|No| DBNONE
        DBIMPACT -->|Sí / Riesgo alto| DBSPEC

        DBSPEC --> DBASSESS

        DBNONE --> HG2
        DBASSESS --> HG2

        HG2 -->|Rework Specs| SPECIFIER
        HG2 -->|Rework Design| DESIGNER
    end

    %% =========================================================
    %% 3. DESCOMPOSICIÓN DE TRABAJO
    %% =========================================================

    subgraph P3["3. DESCOMPOSICIÓN DE TRABAJO"]

        PLANNER["dev-task-planner"]
        DAG["Genera DAG de Tasks"]

        T91["TASK-091<br/>payments-api"]
        T92["TASK-092<br/>orders-api"]
        T93["TASK-093<br/>checkout-mf"]

        C91["Context Package<br/>Skill Set<br/>Repo Profile<br/>Specs + Design"]
        C92["Context Package<br/>Skill Set<br/>Repo Profile<br/>Specs + Design"]
        C93["Context Package<br/>Skill Set<br/>Repo Profile<br/>Specs + Design"]

        HG2 -->|Approved| PLANNER

        PLANNER --> DAG

        DAG --> T91
        DAG --> T92
        DAG --> T93

        T91 --> C91
        T92 --> C92
        T93 --> C93
    end

    %% =========================================================
    %% 4. IMPLEMENTACIÓN
    %% =========================================================

    subgraph P4["4. IMPLEMENTACIÓN"]

        BI1["Backend Implementer<br/>Instance A<br/>payments-api"]
        BI2["Backend Implementer<br/>Instance B<br/>orders-api"]
        FI1["Frontend Implementer<br/>Instance C<br/>checkout-mf"]

        CODE1["Commit / Diff<br/>payments-api"]
        CODE2["Commit / Diff<br/>orders-api"]
        CODE3["Commit / Diff<br/>checkout-mf"]

        MIG1["Migration*<br/>payments"]
        MIG2["Migration*<br/>orders"]

        APPLY["Implementation Evidence<br/>Diffs + Tests locales<br/>TASK IDs + SPEC IDs"]

        C91 --> BI1
        C92 --> BI2
        C93 --> FI1

        BI1 --> CODE1
        BI2 --> CODE2
        FI1 --> CODE3

        BI1 -. "si aplica DB" .-> MIG1
        BI2 -. "si aplica DB" .-> MIG2

        CODE1 --> APPLY
        CODE2 --> APPLY
        CODE3 --> APPLY

        MIG1 --> APPLY
        MIG2 --> APPLY
    end

    %% =========================================================
    %% 5. VERIFICACIÓN INDEPENDIENTE
    %% =========================================================

    subgraph P5["5. VERIFICACIÓN INDEPENDIENTE"]

        INTEGRATION["integration-verifier"]
        VERIFIER["dev-verifier"]

        VERIFY["VERIFY-031<br/><br/>SPEC-061 → PASS / FAIL<br/>SPEC-062 → PASS / FAIL"]

        HG3{"HUMAN GATE #3<br/>Final Review"}

        APPLY --> INTEGRATION
        INTEGRATION --> VERIFIER
        VERIFIER --> VERIFY
        VERIFY --> HG3

        HG3 -->|Rework| PLANNER
    end

    %% =========================================================
    %% 6. GIT / CI / RELEASE
    %% =========================================================

    subgraph P6["6. GIT / CI / RELEASE"]

        MR1["MR-501<br/>payments-api<br/><br/>Implements: TASK-091<br/>Implements: SPEC-061"]

        MR2["MR-502<br/>orders-api<br/><br/>Implements: TASK-092<br/>Implements: SPEC-061"]

        MR3["MR-503<br/>checkout-mf<br/><br/>Implements: TASK-093<br/>Implements: SPEC-062"]

        CI["CI / Pipeline"]

        BUILD["BUILD-8241"]

        RELEASE["RELEASE-3.14.0"]

        INDEX["Registry / Trace Index<br/><br/>TDR → REQ → SPEC → DESIGN<br/>→ TASK → MR → BUILD → RELEASE"]

        HG3 -->|Approved| MR1
        HG3 -->|Approved| MR2
        HG3 -->|Approved| MR3

        MR1 --> CI
        MR2 --> CI
        MR3 --> CI

        CI --> BUILD
        BUILD --> RELEASE
        RELEASE --> INDEX
    end

    %% =========================================================
    %% TRAZABILIDAD TRANSVERSAL
    %% =========================================================

    REQ -. "Implements" .-> SPEC1
    REQ -. "Implements" .-> SPEC2

    SPEC1 -. "Implemented-By" .-> T91
    SPEC1 -. "Implemented-By" .-> T92
    SPEC2 -. "Implemented-By" .-> T93

    DESIGN -. "Based-On" .-> T91
    DESIGN -. "Based-On" .-> T92
    DESIGN -. "Based-On" .-> T93

    T91 -. "Implements" .-> MR1
    T92 -. "Implements" .-> MR2
    T93 -. "Implements" .-> MR3

    VERIFY -. "Verified-By" .-> MR1
    VERIFY -. "Verified-By" .-> MR2
    VERIFY -. "Verified-By" .-> MR3

    %% =========================================================
    %% STYLES
    %% =========================================================

    classDef orchestrator fill:#403522,stroke:#dfbd76,stroke-width:2px,color:#fff;
    classDef agent fill:#1f2a38,stroke:#6cb6ff,stroke-width:1.5px,color:#fff;
    classDef artifact fill:#1d3025,stroke:#7ec699,stroke-width:1.5px,color:#fff;
    classDef gate fill:#302338,stroke:#c792ea,stroke-width:2px,color:#fff;
    classDef task fill:#33281d,stroke:#f0a060,stroke-width:1.5px,color:#fff;
    classDef infrastructure fill:#292929,stroke:#aaaaaa,stroke-width:1.5px,color:#fff;
    classDef database fill:#38231f,stroke:#f7768e,stroke-width:1.5px,color:#fff;

    class ORCH orchestrator;

    class PROPOSER,SPECIFIER,DESIGNER,PLANNER,BI1,BI2,FI1,INTEGRATION,VERIFIER,DBSPEC,SOL_GREEN,SOL_MIX,EXP_EXIST,EXP_MIX agent;

    class TDR,REQ,EXP,PROP,SPEC1,SPEC2,DESIGN,BLUE_GREEN,BLUE_MIX,DBASSESS,VERIFY artifact;

    class HG1,HG2,HG3,TYPE,DBIMPACT gate;

    class T91,T92,T93,C91,C92,C93 task;

    class TRACE,REPORES,SKILLRES,CONTEXT,REG_EXIST,REG_GREEN,REG_MIX,ARCH_GREEN,ARCH_MIX,CI,BUILD,RELEASE,INDEX infrastructure;

    class MIG1,MIG2 database;
```

- Base de datos aparece solo cuando corresponde.

---

# Lo importante: esto NO significa que todo eso ya esté instalado

Aquí hay que ser muy preciso cuando lo expliques.

Yo separaría tres estados:

```
DISEÑADO
    ≠
CONFIGURADO
    ≠
OPERATIVO
```

## Lo que tenemos actualmente

Según lo que hemos venido construyendo:

### 1. SDD base

Ya tenemos el concepto/pipeline de:

```
explore
proposal
spec
design
tasks
apply
verify
```

y la documentación existente explica precisamente que cada fase puede ejecutarse con subagentes y contexto limpio.

Eso es nuestra base.

---

### 2. Skills

Ya trabajaron la estructura de Skills y el Skill Registry.

Conceptualmente:

```
Skill Registry
      │
      ▼
Delegator
      │
      ├── java-spring
      ├── postgresql
      ├── api-contracts
      └── security
             │
             ▼
        subagente
```

La documentación actual indica justamente que el delegador resuelve Skills y las inyecta en el prompt del subagente.

### Eso ya es una pieza real que podemos reutilizar.

---

### 3. Definiciones de agentes

Ya hemos definido los roles que queremos:

```
dev-explorer
dev-proposer
dev-specifier
dev-designer
dev-task-planner

backend-implementer
frontend-implementer

dev-verifier
```

y los especialistas que hemos decidido añadir:

```
solution-architect
database-specialist
project-bootstrap
integration-verifier
```

Pero aquí debes decir:

> **Los roles están diseñados/definidos, pero todavía no significa que exista toda la orquestación automática que muestra el diagrama.**
> 

---

# Entonces, ¿qué estamos construyendo realmente ahora?

Actualmente estamos utilizando la **misma infraestructura/capacidad de Gentle para definir Skills y subagentes**.

Es decir:

```
Gentle actual
│
├── definición de agents
├── definición de Skills
├── SDD existente
│
└── nosotros estamos agregando
    reglas y arquitectura
```

Todavía no tenemos una plataforma externa tipo:

```
"Multiagent Engine v1"
```

corriendo aparte.

Estamos evolucionando el Gentle/SDD existente hasta convertirlo en eso.

---

# Estado real que yo pondría en la documentación

```
┌────────────────────────────────────────────────────┐
│                    YA TENEMOS                      │
├────────────────────────────────────────────────────┤
│ ✅ SDD base                                        │
│ ✅ concepto de subagentes por fase                 │
│ ✅ mecanismo de Skills                             │
│ ✅ Skill Registry                                  │
│ ✅ experiencia previa con Orchestrator QA          │
│ ✅ definición inicial de Agents de Development     │
│ ✅ diseño del flujo Development                    │
│ ✅ diseño de Greenfield                            │
│ ✅ diseño preliminar Database Specialist           │
│ ✅ arquitectura conceptual de trazabilidad         │
└────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────┐
│              ESTÁ DISEÑADO, FALTA INTEGRAR         │
├────────────────────────────────────────────────────┤
│ 🟡 Dev Orchestrator real                           │
│ 🟡 contratos técnicos de cada agente               │
│ 🟡 Context Package                                 │
│ 🟡 Agent Router                                    │
│ 🟡 Repository Resolver                             │
│ 🟡 Traceability Manager                            │
│ 🟡 State/Gate Manager                              │
│ 🟡 Greenfield routing                              │
│ 🟡 DB-specialist routing                           │
└────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────┐
│                    NOS FALTA                       │
├────────────────────────────────────────────────────┤
│ ⬜ Repository Registry                             │
│ ⬜ Architecture Catalog                            │
│ ⬜ Reference Architectures                         │
│ ⬜ Repository Profiles                             │
│ ⬜ Artifact ID standard y  frontmatter schema      │
│ ⬜ Git trailer standard                            │
│ ⬜ persistencia del estado del Orchestrator        │
│ ⬜ recovery / resume                               │
│ ⬜ aislamiento Git multiagente                     │
│ ⬜ integración GitLab                              │
│ ⬜ integración CI                                  │
│ ⬜ BUILD / RELEASE traceability                    │
│ ⬜ validadores automáticos                         │
│ ⬜ piloto end-to-end                               │
└────────────────────────────────────────────────────┘
```

---

# Pasos que nos faltan

Yo seguiría este orden.

## Paso 1 — Congelar la arquitectura de Agents

Primero definir definitivamente:

```
dev-orchestrator

analysis:
  dev-explorer
  dev-proposer
  dev-specifier
  dev-designer
  dev-task-planner

execution:
  backend-implementer
  frontend-implementer

specialists:
  solution-architect
  database-specialist
  project-bootstrap

assurance:
  integration-verifier
  dev-verifier
```

No seguir agregando agentes todavía.

---

# Paso 2 — Especificar el contrato de CADA agente

Esto es lo siguiente que yo haría.

Cada archivo de agente debe indicar:

```
agent:
  id: dev-explorer

responsibility:
  "Discover current system reality"

inputs:
  - REQ
  - Repository Registry
  - Repo Profiles

permissions:
  repository: read-only
  git: read-only
  database: read-only

skills:
  resolved_by: orchestrator

produces:
  artifact: EXP

forbidden:
  - modify_code
  - create_commits
  - change_architecture
```

Y lo mismo para cada agente.

Esto nos falta.

---

# Paso 3 — Definir el contrato universal de delegación

El Orchestrator no debería enviar:

```
Analiza payments-api.
```

Debe enviar algo como:

```
execution_id: EXEC-2026-0042

agent:
  dev-explorer

trace:
  requirement:
    - REQ-042

scope:
  repositories:
    - payments-api

inputs:
  artifacts:
    - REQ-042

skills:
  - sdd-exploration
  - java-spring

repo_profile:
  payments-api

permissions:
  code: read
  git: read

expected_output:
  type: EXP
  id: EXP-042
```

Eso es el:

# Context Package

Y será probablemente una de las piezas más importantes del sistema.

---

# Paso 4 — Definir IDs y trazabilidad

Adoptar lo que planteó tu gerente.

Por ejemplo:

```
TDR-007
REQ-042
EXP-042
PROP-018
SPEC-061
DESIGN-018
DBASSESS-021
TASK-091
VERIFY-031
MR-501
BUILD-8241
RELEASE-3.14.0
```

Y relaciones:

```
Originates-From:
Implements:
Depends-On:
Relates-To:
Supersedes:
Verified-By:
Released-In:
```

---

# Paso 5 — Repository Registry

Construir:

```
payments-api:
  type: backend
  domain: payments
  owner: payments

  stack:
    language: java
    framework: spring

  provides:
    - payments-api

  consumes:
    - customer-api

  database:
    owner: payments

checkout-mf:
  type: microfrontend
  framework: angular
  consumes:
    - payments-api
```

Entonces Impact Discovery no empieza desde cero.

---

# Paso 6 — Repository Profiles

Dentro de cada repo:

```
payments-api/
└── .agent/
    └── repo-profile.yaml
```

Con:

```
stack
arquitectura
comandos
tests
restricciones
DB
dependencias
convenciones
Engram project
```

---

# Paso 7 — Architecture Catalog

Crear la otra fuente:

```
Repository Registry
= qué tenemos

Architecture Catalog
= cómo queremos construir
```

Ejemplo:

```
architecture/
├── spring-rest-service
├── angular-microfrontend
├── event-driven-worker
└── batch-processing
```

Esto habilita el Greenfield real.

---

# Paso 8 — Implementar el Dev Orchestrator

Aquí sí entramos a modificar Gentle.

El Orchestrator tendría componentes lógicos:

```
dev-orchestrator
│
├── Intent Router
├── Repository Resolver
├── Agent Router
├── Skill Resolver
├── Context Builder
├── Traceability Manager
└── State/Gate Manager
```

No necesariamente siete procesos separados.

Pueden comenzar siendo siete responsabilidades dentro de la definición del mismo Orchestrator.

---

# Paso 9 — Añadir State Machine

Algo así:

```
INTAKE
 ↓
DISCOVERY
 ↓
PROPOSAL
 ↓
SCOPE_GATE
 ↓
SPEC_DESIGN
 ↓
TECH_GATE
 ↓
TASK_PLANNING
 ↓
IMPLEMENTATION
 ↓
VERIFY
 ↓
FINAL_GATE
 ↓
READY_FOR_MR
```

Eso permite saber dónde está una ejecución.

---

# Paso 10 — DB Routing

Reglas como:

```
DB impact = none
    ↓
no database-specialist

DB impact = simple
    ↓
backend + DB Skill

DB impact = high-risk
    ↓
database-specialist
```

---

# Paso 11 — Greenfield Routing

```
Existing
   ↓
Impact Discovery

Greenfield
   ↓
Greenfield Discovery
   ↓
Solution Architect
   ↓
Blueprint
   ↓
Architecture Gate
   ↓
Project Bootstrap
```

---

# Paso 12 — Pilotar SIN GitLab

Esto es importante.

El primer piloto podría llegar hasta:

```
REQ
↓
Discovery
↓
Proposal
↓
Specs
↓
Design
↓
Tasks
↓
simulated Apply Plan
↓
Verification Plan
```

sin:

```
push
MR
pipeline
release
```

Así validas la inteligencia de la arquitectura primero.

---

# Paso 13 — Después conectar Git

Cuando SDD funcione correctamente:

```
TASK
 ↓
isolated worktree
 ↓
Implementer
 ↓
commit
```

con:

```
Implements: TASK-091
Implements: SPEC-061
Based-On: DESIGN-018
```

---

# Paso 14 — Después GitLab / CI

Finalmente:

```
Commit
 ↓
MR
 ↓
CI
 ↓
VERIFY
 ↓
BUILD
 ↓
RELEASE
```

No intentaría implementar esto en la primera iteración.

---

# Cómo explicarlo en una reunión

Yo lo diría así:

> **“Hoy no tenemos todavía instalado un sistema multiagente completo independiente. Estamos evolucionando el Gentle/SDD actual, reutilizando su mecanismo de subagentes y Skills. Ya definimos los roles y el proceso objetivo. Lo que sigue es transformar esos conceptos en contratos técnicos: qué recibe cada agente, qué produce, sus permisos, sus Skills, cómo se trazan sus artefactos y cómo el Orchestrator decide qué agente ejecutar. Después incorporamos Repository Registry, Architecture Catalog y Repo Profiles; finalmente integramos Git, CI y releases.”**
> 

Y después muestras:

```
HOY

Gentle
 ├── SDD
 ├── Skills
 └── subagentes

        ↓ evolución

MVP

Gentle
 └── Dev Orchestrator
      ├── Agent Router
      ├── Skill Resolver
      ├── Context Builder
      ├── Trace Manager
      └── subagentes especializados

        ↓ evolución

OBJETIVO

Requirements
   ↓
Multiagent Engineering
   ↓
Git
   ↓
CI
   ↓
Build
   ↓
Release
```

---

# Prompt para que tu agente audite qué existe REALMENTE

Esto también te lo recomiendo mucho antes de presentarlo.

Puedes darle este prompt al Gentle actual:

```
Necesito que audites el estado REAL de la implementación multiagente
- Architecture Catalog
- Repository Profiles
- Context Package
- Agent Router
- Repository Resolver
- Traceability Manager
- State/Gate Manager
- Artifact IDs
- frontmatter
- Git trailers
- Greenfield routing
- database routing
- multi-repo handling
- Git integration
- GitLab integration
- CI integration
- Build/Release traceability
- Engram integration

Para cada elemento entrega:

| Componente | Estado | Archivo/Evidencia | Qué hace hoy | Qué falta |

No asumas que algo está implementado porque aparece en un documento.

Si dices IMPLEMENTED, debes indicar exactamente:
- archivo,
- ruta,
- configuración,
- función/comando que lo usa.

Después genera dos diagramas Mermaid:

1. ARQUITECTURA REAL ACTUAL
   Solo lo que realmente existe.

2. ARQUITECTURA OBJETIVO
   Lo que describen nuestros documentos.

Finalmente genera:

## GAP ANALYSIS

ordenado por prioridad:

P0 - necesario para primer MVP
P1 - necesario para piloto
P2 - evolución posterior

Y termina con:

## RECOMMENDED NEXT 10 IMPLEMENTATION STEPS

No implementes ninguno.
Solo documenta.
```

Ese resultado nos va a servir muchísimo, porque podremos hacer dos diagramas distintos:

```
AS-IS
lo que Gentle realmente hace hoy

TO-BE
lo que queremos que haga
```

Y esa separación evita presentar al gerente como “implementado” algo que en realidad todavía es arquitectura en construcción.

**Ese es exactamente el siguiente paso que yo haría ahora: inventario técnico AS-IS → Gap Analysis → recién comenzar a implementar el Dev Orchestrator.**
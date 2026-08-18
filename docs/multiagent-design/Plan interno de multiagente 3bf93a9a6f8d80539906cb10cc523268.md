# Plan interno de multiagente

Sí. Lo que dijo tu gerente **sí modifica nuestro diseño**, aunque no tanto el proceso SDD como la **infraestructura técnica que sostiene al Orchestrator y los subagentes**.

Hasta ahora diseñamos bastante bien:

```
qué fases existen
qué agente participa
qué Skill necesita
qué repositorio impacta
qué pasa con proyectos nuevos
qué pasa con DB
```

Pero todavía nos falta especificar algo más profundo:

> **Cuál es el contrato técnico entre los agentes, qué artefactos reciben, qué artefactos producen, cómo se identifican, cómo se relacionan, quién puede modificarlos, dónde viven y cómo el Orchestrator sabe qué hacer después.**
> 

Eso es probablemente donde un ingeniero senior empezará a hacer preguntas.

Además, el SDD que ya tienen insiste en separar fases mediante artefactos revisables y contextos limpios.  Y el modelo actual del Orchestrator ya lo plantea como coordinador que delega, no como ejecutor.  Lo que falta ahora es volver eso **un protocolo técnico implementable**.

---

# 1. Qué cambia en nuestro Dev Orchestrator

Antes teníamos conceptualmente:

```
Requirement
    ↓
Dev Orchestrator
    ↓
Discovery
    ↓
...
```

Ahora yo lo modelaría internamente como:

```
                    DEV ORCHESTRATOR
                           │
          ┌────────────────┼─────────────────┐
          │                │                 │
          ▼                ▼                 ▼
   Context Resolver   Trace Resolver    Agent Router
          │                │                 │
          ▼                ▼                 ▼
      qué contexto       qué IDs           qué agente
      necesita           relacionados      ejecutar
```

Es decir, el Orchestrator ya no decide solamente:

> “Ahora toca Explorer.”
> 

También debe saber:

```
execution:
  change_id: CHG-042

  current_artifact:
    id: REQ-042
    status: published

  upstream:
    - TDR-007

  agent:
    dev-explorer

  repositories:
    - payments-api
    - checkout-mf

  skills:
    - sdd-exploration
    - payments-domain

  expected_output:
    type: EXPLORATION
    id: EXP-042

  permissions:
    repository: read-only
    gitlab: forbidden
```

Eso sí es un **contrato de ejecución**.

---

# 2. Yo añadiría un `Trace Context`

Cada delegación debería llevar algo parecido a esto:

Entonces un agente nunca trabaja simplemente con:

> “Diseña la solución.”
> 

Trabaja con:

> “Diseña la solución para `REQ-042`, usando `SPEC-061` y `EXP-042`, y produce `DESIGN-018`.”
> 

Eso cambia bastante la precisión del sistema.

---

```
trace_context:

  change_id: CHG-042

  originates_from:
    - TDR-007

  requirement:
    - REQ-042

  current_phase:
    design

  consumes:
    - SPEC-061
    - EXP-042

  expected_output:
    id: DESIGN-018

  repositories:
    - payments-api
    - checkout-mf
```

# 3. Ya no deberíamos pensar solamente en Agents

La unidad fundamental pasa a ser:

```
Agent
   +
Artifact Contract
   +
Context Package
   +
Permissions
   +
Trace Context
   +
Output Contract
```

Por ejemplo:

```
dev-designer
    +
SPEC-061
    +
Repo Profiles
    +
Architecture Catalog
    +
read-only
    +
produce DESIGN-018
```

---

# 4. Contrato técnico de cada subagente

Aquí es donde creo que debemos profundizar.

## `dev-explorer`

### Responsabilidad

Descubrir **qué existe realmente**.

### Inputs

```
inputs:
  requirement:
    - REQ-042

  repositories:
    - payments-api
    - checkout-mf

  repository_registry: true
  repo_profiles: true

  permissions:
    code: read
    git_history: read
    database_schema: read-if-available
```

### Puede

```
leer código
leer tests
leer documentación
leer historial Git
consultar Repository Registry
consultar Engram
consultar contratos
analizar migrations
```

### No puede

```
editar código
crear arquitectura
crear migrations
hacer commits
crear MRs
```

### Produce

```
artifact:
  id: EXP-042
  type: exploration

  implements:
    - REQ-042

  contains:
    - repositories
    - modules
    - dependencies
    - integrations
    - databases
    - risks
    - unknowns
```

---

# 5. `dev-proposer`

No debería explorar desde cero.

Consume:

```
REQ-042
EXP-042
```

Produce:

```
PROP-018
```

Contrato:

```
artifact:
  id: PROP-018

  based_on:
    - REQ-042
    - EXP-042

  alternatives:
    - option-a
    - option-b
    - option-c

  recommendation:
    selected:

  risks:
  scope:
  out_of_scope:
```

Su responsabilidad no es:

> “hacer arquitectura detallada”.
> 

Es:

> **comparar enfoques y recomendar uno.**
> 

---

# 6. `dev-specifier`

Consume:

```
REQ
Proposal
Exploration
```

Produce uno o varios:

```
SPEC-061
SPEC-062
SPEC-063
```

Ejemplo:

```
id: SPEC-061

implements:
  - REQ-042

derived_from:
  - PROP-018
  - EXP-042

acceptance:
  - given:
    when:
    then:
```

Importante:

**Specifier no decide Spring, PostgreSQL, Kafka, clases, tablas, etc.**

Eso pertenece a Design.

---

# 7. `dev-designer`

Consume:

```
REQ-042
EXP-042
PROP-018
SPEC-061
SPEC-062
Repository Profiles
Architecture Catalog
```

Produce:

```
DESIGN-018
```

Debe contener:

```
id: DESIGN-018

implements:
  - SPEC-061
  - SPEC-062

repositories:
  - payments-api
  - checkout-mf

architecture:

contracts:

database:

security:

observability:

deployment:

rollback:

risks:
```

---

# 8. `solution-architect`

Aquí tenemos que distinguirlo bien del Designer.

Este agente debería aparecer cuando:

```
nuevo proyecto
nuevo microservicio
nuevo microfront
nueva frontera de dominio
cambio transversal importante
nueva tecnología
nuevo modelo de persistencia
```

No para cualquier ticket.

Consume:

```
REQ
Exploration
Repository Registry
Architecture Catalog
Infrastructure Catalog
Domain topology
```

Produce por ejemplo:

```
BLUEPRINT-012
```

que puede decir:

```
id: BLUEPRINT-012

originates_from:
  - REQ-088

project:
  reconciliation-api

reference_architecture:
  spring-rest-service

domain:
  banking

database:
  decision:
  ownership:

integration:

infrastructure:
```

---

# 9. `database-specialist`

También necesita contrato propio.

No solamente:

> “revisa la DB”.
> 

Consume:

```
SPEC
DESIGN
DB schema
migrations actuales
volumen aproximado
ownership
deployment model
```

Produce:

```
DBASSESS-021
```

Ejemplo:

```
id: DBASSESS-021

related_to:
  - DESIGN-018
  - SPEC-061

impact:
  schema_change: true
  migration: true
  data_migration: false

ownership:
  database: payments
  owner: payments-api

compatibility:
  rolling_deployment: safe

risks:
  - locking

recommendations:
```

Muy importante:

**No necesariamente crea migrations.**

Puede ser principalmente un agente de análisis/revisión.

---

# 10. `dev-task-planner`

Este agente empieza a ser mucho más interesante con trazabilidad.

Consume:

```
SPEC
DESIGN
DB Assessment
Blueprint si aplica
```

Produce:

```
TASK-091
TASK-092
TASK-093
```

Por ejemplo:

```
id: TASK-091

implements:
  - SPEC-061

based_on:
  - DESIGN-018

repository:
  payments-api

capability:
  backend-implementer

depends_on:
  - TASK-090

skills:
  - java-spring
  - postgresql

expected_changes:
  - API
  - service

acceptance:
  - SPEC-061
```

Esto le permite al Orchestrator saber automáticamente:

```
TASK-091
   ↓
backend
   ↓
payments-api
   ↓
java-spring + postgres
```

---

# 11. `backend-implementer`

Aquí también necesitamos establecer límites.

Consume:

```
TASK
SPEC relacionadas
fragmento relevante de DESIGN
Repo Profile
Skills
```

No debería recibir automáticamente:

```
todas las conversaciones
todo Engram
todo el Architecture Catalog
todo el sistema
```

Eso contradice justamente el modelo de contexto preciso bajo demanda que ya tienen con Skills. El Skill Registry actual está pensado para que el delegador resuelva reglas compactas e inyecte únicamente lo necesario al subagente.

Output futuro:

```
commit / diff
```

con metadata:

```
Implements: TASK-091
Implements: SPEC-061
Based-On: DESIGN-018
```

---

# 12. `frontend-implementer`

Mismo principio.

Pero sus Skills/context podrían ser:

```
Angular
UI conventions
API contracts
design system
testing
microfrontend architecture
```

Produce otro commit/MR:

```
Implements: TASK-092
Implements: SPEC-061
```

---

# 13. `dev-verifier`

Este agente necesita ser particularmente estricto.

Consume:

```
SPEC
DESIGN
TASKS
diff
tests
build evidence
```

Produce:

```
VERIFY-031
```

Ejemplo:

```
id: VERIFY-031

verifies:
  - SPEC-061
  - SPEC-062

implementation:
  - MR-501
  - MR-502

results:
  SPEC-061: PASS
  SPEC-062: FAIL

evidence:
```

Y algo importante:

```
dev-verifier
≠
backend-implementer
```

No debería ser la misma instancia/contexto.

---

# 14. `project-bootstrap`

Solo para proyectos nuevos.

Consume:

```
BLUEPRINT
Architecture Catalog
template elegido
Git standards
```

Produce conceptualmente:

```
nuevo repository skeleton
repo-profile
README
CI skeleton
Dockerfile
project conventions
```

Y después:

```
Repository Registry
```

debe incorporar el proyecto nuevo.

---

# 15. Nuestro mapa de agentes se empieza a ver así

Esto ya no es proceso.

Es **organización funcional y contratos**:

```
                         DEV ORCHESTRATOR
                                │
             ┌──────────────────┼──────────────────┐
             │                  │                  │
             ▼                  ▼                  ▼
       ANALYSIS             EXECUTION          SPECIALISTS
             │                  │                  │
      dev-explorer       backend-impl       solution-architect
      dev-proposer       frontend-impl      database-specialist
      dev-specifier                          project-bootstrap
      dev-designer
      dev-task-planner
             │                  │                  │
             └──────────────────┼──────────────────┘
                                │
                         dev-verifier
```

Todos producen artefactos identificables.

---

# 16. El cambio más importante al Orchestrator

Yo ahora lo definiría con **7 responsabilidades técnicas**:

```
DEV ORCHESTRATOR

1. Intent Router
2. Repository Resolver
3. Agent Router
4. Skill Resolver
5. Context Builder
6. Traceability Manager
7. State / Gate Manager
```

No programa.

No diseña.

No verifica.

Coordina estas siete cosas.

---

# 17. `Intent Router`

Determina:

```
bug
feature
refactor
project-new
database-change
architecture-change
security-change
```

Y especialmente:

```
existing
greenfield
mixed
```

---

# 18. `Repository Resolver`

Responde:

```
¿qué repositorios posiblemente participan?
```

usando:

```
Repository Registry
dependency map
domain map
```

No debería escanear 100 repos completos inicialmente.

# 19. `Agent Router`

Responde:

```
¿Qué agente necesito?
```

Ejemplo:

```
impact:
  backend: true
  frontend: true
  database_risk: high
```

Entonces:

```
backend-implementer
frontend-implementer
database-specialist
```

---

# 20. `Skill Resolver`

Responde:

```
¿Cómo debe trabajar ese agente?
```

Por ejemplo:

```
backend-implementer
+
java-spring
+
postgresql
+
api-contract
+
secure-coding
```

Esto coincide directamente con el patrón de resolución de Skills que ya tienen.

---

# 21. `Context Builder`

Produce algo como:

```
context_package:

  agent:
    backend-implementer

  task:
    TASK-091

  artifacts:
    specs:
      - SPEC-061

    design:
      - DESIGN-018

  repository:
    payments-api

  skills:
    - java-spring
    - postgresql

  trace:
    requirement:
      - REQ-042
```

---

# 22. `Traceability Manager`

Esta es la nueva pieza derivada directamente de lo que planteó tu gerente.

Debe controlar:

```
IDs
relationships
artifact states
immutability
supersession
```

Por ejemplo:

```
TASK-091
       ↓ implements
SPEC-061
       ↓ implements
REQ-042
       ↓ originates-from
TDR-007
```

---

# 23. `State / Gate Manager`

Debe saber:

```
DRAFT
READY_FOR_REVIEW
APPROVED
PUBLISHED
SUPERSEDED
REJECTED
```

y:

```
¿puedo crear tasks si DESIGN todavía es draft?
```

Probablemente:

```
NO
```

Pero esto todavía tenemos que definir.

Y aquí empiezan justamente **las preguntas que todavía tenemos abiertas**.

---

# Preguntas técnicas que debemos cerrar

Yo llevaría esta lista a la próxima reunión. No son preguntas decorativas; cada grupo representa una decisión de arquitectura pendiente.

### Identidad y artefactos

1. ¿Quién genera `REQ-042`, `SPEC-061`, `DESIGN-018`, etc.?
2. ¿Los IDs son globales o por repositorio?
3. ¿Puede existir `SPEC-001` en dos repos diferentes?
4. ¿El ID lleva dominio? `PAY-SPEC-042`.
5. ¿Dónde vive el contador/generador de IDs?
6. ¿Un artefacto puede cambiar de ID?
7. ¿Cuándo exactamente se considera “publicado”?
8. ¿Quién puede publicar?
9. ¿Una vez publicado es físicamente read-only o solo una convención?
10. ¿Cómo corregimos un typo en un artefacto publicado?
11. ¿Cuándo usamos `Supersedes` y cuándo nueva versión?
12. ¿Cómo representamos artefactos retirados?

### Relaciones

1. ¿Cuál será el vocabulario oficial?
2. ¿`Implements`, `Depends-On`, `Closes`, `Relates-To`, `Supersedes`?
3. ¿Podemos tener relaciones arbitrarias?
4. ¿Quién valida que el ID referenciado exista?
5. ¿Qué pasa si `TASK-091` referencia `SPEC-999` inexistente?
6. ¿Se permiten ciclos?
7. ¿Cómo se representa N:N?
8. ¿Una Task puede implementar múltiples Specs?
9. ¿Una Spec puede provenir de múltiples Requirements?

### Almacenamiento

1. ¿Dónde viven los TDR?
2. ¿Dónde viven los Requirements?
3. ¿Dónde viven Specs y Designs?
4. ¿En el repo del servicio?
5. ¿En un repo central de engineering?
6. ¿Qué hacemos en cambios multi-repo?
7. ¿Quién es owner del artefacto?
8. ¿Dónde vive el Blueprint de un proyecto nuevo?
9. ¿Los artefactos se escriben como Markdown?
10. ¿Usamos YAML frontmatter?
11. ¿Definiremos JSON Schema para validarlo?

### Orchestrator

1. ¿Dónde está el estado actual de una ejecución?
2. ¿Puede reiniciarse y continuar?
3. ¿Cómo sabe cuál fue el último artefacto?
4. ¿Lee Git?
5. ¿Lee Engram?
6. ¿Lee ambos?
7. ¿Qué fuente tiene prioridad?
8. ¿Puede ejecutar agentes en paralelo?
9. ¿Cuántos?
10. ¿Quién controla timeout?
11. ¿Qué pasa si un subagente falla?
12. ¿Puede reintentarlo?
13. ¿Cuántas veces?
14. ¿Qué pasa si dos agentes producen resultados contradictorios?
15. ¿Cuándo escala a humano?

### Contexto

1. ¿Cuánto contexto recibe cada agente?
2. ¿Quién selecciona archivos?
3. ¿Quién selecciona memorias de Engram?
4. ¿Cómo evitamos meter todo el repo?
5. ¿Cómo detectamos información stale?
6. ¿El agente debe poder buscar más contexto?
7. ¿Hasta dónde?
8. ¿Puede leer otros repos?
9. ¿Qué permisos tiene?
10. ¿Cómo evitamos context leakage entre proyectos?

### Repository Registry

1. ¿Quién mantiene el Registry?
2. ¿Manual o automático?
3. ¿Qué campos son obligatorios?
4. ¿Stack?
5. ¿Dominio?
6. ¿Owner?
7. ¿APIs?
8. ¿Eventos?
9. ¿DB?
10. ¿Infra?
11. ¿Dependencias?
12. ¿Cómo se actualiza cuando aparece un repo nuevo?
13. ¿Cómo detectamos que el registry quedó desactualizado?

### Architecture Catalog

1. ¿Quién aprueba una Reference Architecture?
2. ¿Cuáles tenemos realmente?
3. ¿Java Spring REST?
4. ¿Worker?
5. ¿Microfront?
6. ¿Event-driven?
7. ¿Qué repos son “golden examples”?
8. ¿Qué repos explícitamente NO deben copiarse?
9. ¿Dónde viven templates?
10. ¿Cómo versionamos un template?

### Greenfield

1. ¿Quién decide definitivamente crear un repo nuevo?
2. ¿Puede decidirlo un agente?
3. ¿Requiere Architecture Gate?
4. ¿Quién crea el repo?
5. ¿Quién asigna ownership?
6. ¿Quién define nombre?
7. ¿Quién define el dominio?
8. ¿Quién decide DB?
9. ¿Quién decide deployment target?
10. ¿Bootstrap puede ejecutar GitLab o inicialmente solo proponer?

### Base de datos

1. ¿Quién determina ownership de datos?
2. ¿Tenemos database/schema por servicio?
3. ¿Compartimos bases?
4. ¿Qué patrón corporativo queremos?
5. ¿Puede un servicio leer tablas de otro?
6. ¿O solamente API/eventos?
7. ¿Quién aprueba una nueva DB?
8. ¿Qué define “DB change de alto riesgo”?
9. ¿Cuándo activamos `database-specialist`?
10. ¿Quién genera migration?
11. ¿Backend Implementer o DB Specialist?
12. ¿Quién la revisa?
13. ¿Cómo trazamos migration → TASK → SPEC?
14. ¿Cómo hacemos rollback?
15. ¿Cómo manejamos breaking migrations?
16. ¿Cómo verificamos rolling deployment?

### Git

1. ¿Branch por Change o por Task?
2. ¿Los agentes pueden hacer commits?
3. ¿Pueden hacer push?
4. ¿Quién crea MR?
5. ¿Qué trailers son obligatorios?
6. ¿CI valida trailers?
7. ¿Un commit puede existir sin `Implements:`?
8. ¿Cómo trazamos squash commits?
9. ¿Qué hacemos con cherry-picks?
10. ¿Cómo trabajamos múltiples agentes sobre un repo?
11. ¿Branches separados?
12. ¿Git worktrees?
13. ¿Quién resuelve conflictos?

### Merge Requests

1. ¿MR tiene ID propio de trazabilidad o usamos el ID GitLab?
2. ¿Qué metadata debe contener?
3. ¿`Implements: TASK-X`?
4. ¿`Closes: REQ-X`?
5. ¿Quién valida la cobertura Specs → Tasks → MR?
6. ¿Puede mergearse una MR sin Verify?
7. ¿Qué Human Gate existe?

### Build y Release

1. ¿Cómo obtiene BUILD un ID?
2. ¿GitLab pipeline ID es suficiente?
3. ¿Build referencia MRs o commits?
4. ¿Release referencia Builds?
5. ¿Cómo relacionamos varios repos con una release?
6. ¿Tenemos release coordinada o independiente por servicio?
7. ¿Dónde se indexan los IDs?
8. ¿Cómo respondemos “qué release implementó REQ-042”?

### Verification

1. ¿Verifier puede ejecutar pruebas?
2. ¿O solamente analizar resultados?
3. ¿Puede modificar código?
4. Yo diría **no**.
5. ¿Cómo se registra evidencia?
6. ¿Existe `VERIFY-XXX`?
7. ¿Una SPEC sin evidencia puede cerrarse?
8. ¿Qué significa PASS exactamente?
9. ¿Quién aprueba excepciones?

### Skills

1. ¿Quién mantiene Skills?
2. ¿Qué es global?
3. ¿Qué es domain-specific?
4. ¿Qué es repository-specific?
5. ¿Quién decide qué Skills inyectar?
6. ¿Cómo versionamos Skills?
7. ¿El artefacto debe registrar qué versión de Skill se utilizó?
8. ¿Qué pasa cuando una Skill cambia después de implementar?

### Agents

1. ¿Los agentes son definiciones versionadas?
2. ¿Debemos registrar su versión?
3. ¿Qué modelo puede usar cada uno?
4. ¿Qué herramientas tiene cada uno?
5. ¿Qué permisos?
6. ¿Qué presupuesto/token limit?
7. ¿Qué timeout?
8. ¿Qué output schema?
9. ¿Quién valida output?
10. ¿Puede un subagente llamar a otro?
11. Nuestra propuesta inicial es **no**.
12. ¿Qué agentes son obligatorios?
13. ¿Cuáles condicionales?

### Engram

1. ¿Qué guardamos en Engram?
2. ¿Qué jamás debe existir solo en Engram?
3. ¿Guardamos decisiones?
4. ¿Guardamos artefactos completos?
5. ¿O solo contexto/resúmenes?
6. ¿Cómo referenciamos IDs de Git desde Engram?
7. ¿Cómo evitamos que memoria antigua contradiga artefactos publicados?

### Seguridad

1. ¿Qué agentes tienen acceso a secrets?
2. Idealmente ninguno salvo necesidad explícita.
3. ¿Cómo prevenimos que un agente escriba secretos en Markdown?
4. ¿Qué repos puede leer?
5. ¿Qué repos puede escribir?
6. ¿Quién controla permisos GitLab?
7. ¿Tenemos sandbox?

### Auditoría

1. ¿Registramos cada ejecución de agente?
2. ¿Qué prompt recibió?
3. ¿Qué Skills?
4. ¿Qué archivos?
5. ¿Qué artefactos produjo?
6. ¿Qué modelo?
7. ¿Cuánto costó?
8. ¿Quién aprobó?
9. ¿Cuándo?

### Concurrencia

1. ¿Dos agentes pueden trabajar el mismo repo?
2. ¿La misma Task?
3. ¿El mismo archivo?
4. ¿Cómo bloqueamos conflictos?
5. ¿Cómo coordinamos dependencias?
6. ¿Qué ocurre si Backend finaliza antes que la Spec cambie?

### Recuperación

1. ¿Qué pasa si Orchestrator muere a mitad?
2. ¿Puede reconstruir estado desde IDs?
3. ¿Cuál es la fuente de verdad del estado?
4. ¿Git?
5. ¿Un manifest?
6. ¿Engram?
7. ¿Cómo retomamos un Change?

---

# Pero no debemos resolver 200 cosas antes de empezar

Esta es quizá la conclusión más importante.

Yo convertiría todo esto en **decisiones por niveles**.

### Nivel 1 — necesario para MVP

Cerrar solamente:

```
IDs
tipos de artefacto
frontmatter
relaciones
states
contrato de cada agente
Repository Profile
Skill resolution
Context Package
Git trailers
Human Gates
```

### Nivel 2

Después:

```
CI validation
MR integration
Build/Release traceability
multi-repo
database specialist
greenfield bootstrap
```

### Nivel 3

Más adelante:

```
indexador global
dashboards
graph visualization
cost tracking
advanced orchestration
multi-domain orchestrators
```

---

# Cómo quedaría nuestro MVP técnicamente

```
REQ-042
   │
   ▼
DEV ORCHESTRATOR
   │
   ├── Trace Context
   ├── Repository Resolver
   ├── Skill Resolver
   ├── Context Builder
   └── Agent Router
          │
          ▼
     dev-explorer
          │
        EXP-042
          │
          ▼
      dev-proposer
          │
       PROP-018
          │
          ▼
   ┌───────────────┐
   │               │
   ▼               ▼
SPECIFIER       DESIGNER
   │               │
SPEC-061       DESIGN-018
   │               │
   └───────┬───────┘
           ▼
     task-planner
           │
     TASK-091..093
           │
     ┌─────┼─────┐
     ▼     ▼     ▼
 Backend Front  DB*
           │
           ▼
          MR
           │
           ▼
        VERIFIER
           │
       VERIFY-031
           │
           ▼
      BUILD / RELEASE
```

`DB*` solo aparece cuando el impacto lo exige.

Y cada flecha ya no significa únicamente:

> “ahora viene la siguiente fase.”
> 

Significa:

> **“el siguiente agente recibe IDs concretos, artefactos concretos, permisos concretos y tiene obligación de producir otro artefacto con relaciones de trazabilidad explícitas.”**
> 

Ahí es donde nuestro diseño pasa de ser un **diagrama bonito de proceso** a una **arquitectura multiagente implementable**.

Yo diría que ese es exactamente el siguiente documento que nos falta: **Agent Technical Contracts + Artifact & Traceability Specification**. Con eso podremos responder casi todas las preguntas que probablemente te haga el ingeniero antes de comenzar a modificar Gentle-AI.
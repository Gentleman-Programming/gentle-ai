# Propuesta: Despliegue Multi-Rol y Barrera de Sincronización en SDD (INC-03)

## Propósito (Intent)

En equipos de ingeniería reales y en entornos colaborativos con múltiples agentes de IA especializados, una solución de software compleja casi nunca es implementada de forma monolítica por un solo perfil. Un diseño arquitectónico habitualmente requiere el trabajo concurrente de responsabilidades diferenciadas (por ejemplo: `backend`, `frontend`, `qa`, `devops` o `database`), cada una operando sobre repositorios o subcarpetas específicas según lo definido en `axiom.yaml`.

Actualmente, los arneses de SDD asumen un flujo lineal estricto con un único archivo `tasks.md`, un único `apply-progress.md` y un único `verify-report.md`. Esto genera colisiones, pérdida de trazabilidad de quién es responsable de qué y la imposibilidad de paralelizar el desarrollo entre distintos agentes o desarrolladores.

El objetivo de este incremento es dotar al runtime de **Axiom** del mecanismo nativo de **Fan-Out y Barrera de Sincronización (Fan-In)**:

1. **Declaración formal de Roles en `Design`:** Identificación explícita en `design.md` de qué roles de la topología `axiom.yaml` participan en la ejecución del cambio.
2. **Desglose Desacoplado de Tareas (`tasks.<rol>.md`):** División canónica de tareas para cada rol participante, permitiendo que cada perfil ejecute su fase de `Tasks` y `Apply` sin colisionar con los demás.
3. **Seguimiento e Informes Aislados de Verificación (`verify-report.<rol>.md`):** Verificación independiente de cada flujo de trabajo según sus propios comandos de test y build.
4. **Barrera de Sincronización en `Archive`:** Regla estricta del motor de Axiom que bloquea el cierre del cambio hasta que todos los roles obligatorios hayan completado y verificado con éxito su respectivo alcance.
5. **Comandos CLI `axiom role`:** Subcomandos para listar roles asignados, consultar su progreso y validar la barrera de sincronización en terminal.

---

## Alcance (Scope)

### Dentro de Alcance (In Scope)

- **Paquete de dominio en el runtime de Axiom (`internal/multirole/`):**
  - `types.go`: Modelos Go para representar los roles participantes en un cambio con su política de compuerta `gate_policy` (`blocking`, `deferred`, `optional`), tareas asignadas, estados de ejecución (`pending`, `in_progress`, `completed`, `verified`, `blocked`), tareas diferidas con trazabilidad (`DeferredTask`) y el informe de la barrera de sincronización (`BarrierReport`).
  - `detector.go`: Extracción y parseo de roles declarados en el documento `design.md` (bloque YAML en `## Roles Participantes` con su respectiva `gate_policy`).
  - `barrier.go`: Motor determinista de evaluación de la barrera de sincronización:
    - Exige que todos los roles `blocking` completen sus tareas y superen su verificación con `pass`.
    - Permite que los roles `deferred` (como E2E) continúen a su propio ritmo sin bloquear la entrega de la funcionalidad.
    - Extrae y formatea las tareas diferidas no concluidas vinculándolas al incremento de origen (`[Ref: <cambio>]`) para migrarlas al incremento acumulativo de QA (`openspec/changes/e2e-cumulative/tasks.md`).
- **Integración CLI en el binario de Axiom (`cmd/axiom/main.go`):**
  - `axiom role list [--change <nombre>]`: Muestra los roles declarados, su política (`blocking`, `deferred`, `optional`) y repositorios asignados.
  - `axiom role status [--change <nombre>]`: Reporta el estado de avance de cada rol (`Tasks`, `Apply`, `Verify`).
  - `axiom role barrier [--change <nombre>] [--migrate-deferred]`: Evalúa la barrera de sincronización antes de abrir PR / mergear a `main`, con opción de exportar/migrar las tareas diferidas pendientes.
- **Suite de pruebas unitarias exhaustiva:** Pruebas en `internal/multirole/...` cubriendo detección de roles en `design.md`, evaluación de barreras con políticas `blocking`, `deferred` y `optional`, migración referenciada de tareas diferidas y consistencia con `axiom.yaml`.

### Fuera de Alcance (Out of Scope)

- Visualización interactiva en tablero web local (`axiom ui`) — objeto del **INC-04**.
- Minería de skills y asignación heurística de skills a los roles — objeto del **INC-05**.
- Modificaciones al arnés metodológico de desarrollo de Gentle AI.

---

## Capacidades (Capabilities)

### Nuevas Capacidades

- `multi-role-fan-out-engine`: Motor en `internal/multirole` para desacoplar tareas y verificaciones por rol (`tasks.<rol>.md`, `verify-report.<rol>.md`) y gobernar políticas de compuerta (`gate_policy`).
- `sdd-synchronization-barrier`: Motor de evaluación que valida que todos los roles obligatorios (`blocking`) hayan alcanzado el estado de verificación exitosa y extrae las tareas diferidas (`deferred`) con referencia de origen.
- `axiom-cli-role`: Subcomandos CLI `axiom role [list|status|barrier]` para inspección, control de ejecución concurrente y migración de tareas diferidas.

---

## Enfoque de Implementación (Approach)

1. **Definición de modelos de datos (`internal/multirole/types.go`):**
   - Estructura `RoleAssignment` (rol, obligatorio u opcional, repositorios afectados, entregables esperados).
   - Estructura `RoleProgress` (tareas totales, tareas completadas, estado de apply, veredicto de verify).
   - Estructura `BarrierReport` (satisfactoria, roles evaluados, bloqueos o pendientes).
2. **Parser y Detector de Roles (`internal/multirole/detector.go`):**
   - Lectura de `design.md` reconociendo la sección `## Roles Participantes` o bloque Frontmatter/YAML de roles.
   - Validación cruzada con `axiom.yaml` asegurando que los roles declarados existan en la topología.
3. **Motor de Barrera de Sincronización (`internal/multirole/barrier.go`):**
   - Inspección del directorio `openspec/changes/<cambio>/`.
   - Para cada rol declarado, busca:
     - `tasks.<rol>.md` (o sección de tareas del rol en mono-archivo).
     - `apply-progress.<rol>.md`.
     - `verify-report.<rol>.md`.
   - Si algún rol obligatorio no tiene su reporte de verificación en `pass`, la barrera rechaza el archivado.
4. **Comandos CLI (`cmd/axiom/main.go`):**
   - Subcomando `axiom role [list|status|barrier]`.
5. **Pruebas y Verificación:**
   - Pruebas unitarias completas guiadas por tabla en `internal/multirole/multirole_test.go`.
   - Validación con `go test` y compilación de `axiom.exe`.

---

## Áreas Afectadas (Affected Areas)

| Área / Archivo | Impacto | Descripción |
| :--- | :--- | :--- |
| `internal/multirole/types.go` | Nuevo | Modelos de asignación de roles, progreso y reporte de barrera |
| `internal/multirole/detector.go` | Nuevo | Extracción y validación de roles participantes desde `design.md` |
| `internal/multirole/barrier.go` | Nuevo | Motor de evaluación determinista de la barrera de sincronización |
| `internal/multirole/multirole_test.go` | Nuevo | Batería de pruebas unitarias exhaustiva |
| `cmd/axiom/main.go` | Modificado | Incorporación de comandos CLI `axiom role list/status/barrier` |

---

## Riesgos y Mitigaciones (Risks)

- **Riesgo:** Incompatibilidad con cambios simples que solo involucran a un rol único y desean mantener el archivo canónico `tasks.md` estándar.  
  *Mitigación:* Si `design.md` no declara múltiples roles explícitos o solo declara 1 rol principal, el motor adopta como fallback el archivo estándar `tasks.md` y `verify-report.md`, garantizando compatibilidad 100% retroactiva.
- **Riesgo:** Un rol secundario bloquea indefinidamente el cierre del cambio.  
  *Mitigación:* Se permite marcar roles como opcionales (`optional: true`) en `design.md`; solo los roles con `required: true` (por defecto) son mandatorios para superar la barrera de sincronización.

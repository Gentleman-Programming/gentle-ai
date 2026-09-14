# Especificación de Requerimientos: Despliegue Multi-Rol y Barrera de Sincronización (INC-03)

## Propósito

Definir de forma rigurosa y verificable los requerimientos funcionales y escenarios BDD para la declaración de responsabilidades concurrentes en `design.md` mediante políticas de compuerta (`gate_policy`), el desglose de tareas por rol (`tasks.<rol>.md`), el reporte de verificación independiente (`verify-report.<rol>.md`), la barrera de sincronización en `Archive`, la trazabilidad y migración de tareas diferidas (ej. E2E) a un incremento acumulativo, y los subcomandos de la CLI `axiom role`.

---

## 1. Capacidad: `multi-role-fan-out-engine`

El paquete `internal/multirole` provee la detección de roles en el diseño, la gobernanza de políticas de compuerta y la gestión del ciclo de tareas desacoplado.

### Requirement: Declaración y Detección de Roles con Políticas de Compuerta en Design (REQ-1.1)

El motor DEBE ser capaz de parsear el documento `design.md` del cambio activo y extraer la lista de roles asignados a la implementación, identificando:
- Identificador del rol (`role`).
- Política de compuerta (`gate_policy`: uno de `blocking`, `deferred`, `optional`; por defecto `blocking`).
- Repositorios asociados (`repositories`: lista de rutas relativas).
- Entregables esperados (`deliverables`: lista de archivos o módulos).

Si `design.md` no contiene una declaración explícita de roles, el motor DEBE operar en modo de compatibilidad unificada asumiendo el rol principal del workspace con política `blocking`.

#### Scenario: Detección exitosa de múltiples roles con distintas políticas
- **DADO** un archivo `design.md` con una sección `## Roles Participantes` declarando los roles `backend` (`gate_policy: blocking`) y `e2e` (`gate_policy: deferred`)
- **CUANDO** el extractor analiza el documento
- **ENTONCES** se identifican ambos roles con sus respectivas políticas
- **Y** no se reporta ningún error

#### Scenario: Validación cruzada de roles contra axiom.yaml
- **DADO** un diseño que declara el rol `database`
- **Y** una configuración `axiom.yaml` que no incluye dicho rol
- **CUANDO** se ejecuta la validación de coherencia
- **ENTONCES** se retorna un error indicando que el rol no existe en el espacio de trabajo

#### Scenario: Modo retrocompatible para diseño de rol único
- **DADO** un archivo `design.md` estándar sin sección explícita de roles
- **CUANDO** el detector procesa el cambio
- **ENTONCES** genera una asignación única con política `blocking` para el rol principal del workspace
- **Y** asocia el archivo canónico `tasks.md`

---

### Requirement: Desglose Desacoplado de Tareas por Rol (REQ-1.2)

El motor DEBE soportar la lectura y cómputo de progreso de tareas atomizadas por rol en archivos independientes con nomenclatura `tasks.<rol>.md` (ej. `tasks.backend.md`, `tasks.frontend.md`, `tasks.e2e.md`).

#### Scenario: Cómputo de progreso por rol
- **DADO** un archivo `tasks.backend.md` con 5 tareas totales y 3 marcadas con `[x]`
- **CUANDO** se calcula el progreso del rol
- **ENTONCES** el motor reporta 5 tareas totales, 3 completadas, 2 pendientes y porcentaje de avance del 60%

---

### Requirement: Informes de Verificación Aislados por Rol (REQ-1.3)

Cada rol participante DEBE emitir su informe de verificación independiente bajo la nomenclatura `verify-report.<rol>.md` (ej. `verify-report.backend.md`, `verify-report.frontend.md`).

#### Scenario: Verificación individual de rol
- **DADO** un archivo `verify-report.frontend.md` con veredicto `pass`
- **CUANDO** el motor inspecciona la evidencia del rol
- **ENTONCES** el rol `frontend` es evaluado como verificado exitosamente

---

## 2. Capacidad: `sdd-synchronization-barrier`

El evaluador de barrera garantiza la coherencia arquitectónica y funcional antes de autorizar el cierre del cambio en la fase `Archive` y la apertura de PRs.

### Requirement: Evaluación de la Barrera de Sincronización con Gate Policy (REQ-2.1)

El motor DEBE evaluar la barrera de sincronización (*Fan-In*) examinando el estado de todos los roles participantes declarados en el diseño:
1. Para que la barrera sea superada (`Satisfied = true`), TODOS los roles con `gate_policy: blocking` DEBEN tener sus tareas al 100% completadas y su informe de verificación con veredicto `pass`.
2. Los roles con `gate_policy: deferred` (ej. `e2e`) NO bloquean la barrera si tienen tareas pendientes, pero se reportan como advertencia (`Warning`).
3. Los roles con `gate_policy: optional` NO bloquean la barrera.

#### Scenario: Barrera superada con roles bloqueantes conformes y rol diferido en progreso
- **DADO** un cambio con roles `backend` (`blocking`) y `frontend` (`blocking`) con 100% tareas y verificación `pass`
- **Y** un rol `e2e` (`deferred`) con 2 de 5 tareas completadas
- **CUANDO** se evalúa la barrera de sincronización
- **ENTONCES** el resultado es aprobado (`Satisfied: true`)
- **Y** se emite una advertencia indicando que `e2e` tiene tareas diferidas pendientes

#### Scenario: Barrera denegada por rol bloqueante con tareas pendientes
- **DADO** un cambio donde el rol `backend` (`blocking`) completó todo pero `frontend` (`blocking`) tiene tareas incompletas
- **CUANDO** se evalúa la barrera
- **ENTONCES** el resultado es rechazado (`Satisfied: false`)
- **Y** el informe detalla que `frontend` no ha concluido sus tareas obligatorias

#### Scenario: Barrera denegada por verificación fallida en rol bloqueante
- **DADO** un rol `backend` (`blocking`) cuyo `verify-report.backend.md` tiene veredicto `fail`
- **CUANDO** se evalúa la barrera
- **ENTONCES** el resultado es rechazado (`Satisfied: false`)
- **Y** se reporta el fallo de verificación del rol

---

### Requirement: Trazabilidad y Migración de Tareas Diferidas (REQ-2.2)

El motor DEBE ser capaz de extraer las tareas no concluidas de roles con política `deferred` al momento de archivar y formatearlas para su migración al incremento acumulativo continuo (`openspec/changes/e2e-cumulative/tasks.md`), preservando el vínculo de trazabilidad con el incremento de origen (`[Ref: <cambio>]`).

#### Scenario: Formateo de tarea diferida con referencia al incremento archivado
- **DADO** un rol `e2e` en el cambio `inc-02-auth` con una tarea pendiente `- [ ] Probar flujo OAuth2 con Google`
- **CUANDO** el motor extrae las tareas diferidas
- **ENTONCES** la tarea generada incluye la referencia `[Ref: inc-02-auth] Probar flujo OAuth2 con Google`
- **Y** vincula la ruta de la especificación archivada correspondiente

---

## 3. Capacidad: `axiom-cli-role`

La interfaz CLI de Axiom proporciona los comandos para inspeccionar, operar y consultar la ejecución de los roles concurrentes.

### Requirement: Subcomando axiom role list (REQ-3.1)

El comando `axiom role list [--change <nombre>]` DEBE listar en terminal los roles asignados a la implementación del cambio, indicando su política de compuerta (`gate_policy`) y sus repositorios asignados.

#### Scenario: Listado de roles participantes
- **DADO** un cambio activo con roles declarados
- **CUANDO** se ejecuta `axiom role list`
- **ENTONCES** la terminal imprime la tabla de roles participantes con su política (`blocking`, `deferred`, `optional`) y finaliza con código `0`

---

### Requirement: Subcomando axiom role status (REQ-3.2)

El comando `axiom role status [--change <nombre>]` DEBE mostrar el resumen del avance de cada rol (`Tareas completadas / totales`, estado de `Apply` y veredicto de `Verify`).

#### Scenario: Consulta de estado de roles
- **DADO** un cambio con ejecución multi-rol en marcha
- **CUANDO** se ejecuta `axiom role status`
- **ENTONCES** la salida muestra el desglose por cada rol y finaliza con código `0`

---

### Requirement: Subcomando axiom role barrier (REQ-3.3)

El comando `axiom role barrier [--change <nombre>] [--migrate-deferred]` DEBE evaluar la barrera de sincronización en terminal y retornar el código de salida correspondiente (`0` si está superada, `1` si hay bloqueos en roles `blocking`), reportando advertencias para tareas diferidas.

#### Scenario: Barrera aprobada en terminal con advertencia diferida
- **DADO** un cambio con roles bloqueantes verificados y un rol diferido con tareas pendientes
- **CUANDO** se ejecuta `axiom role barrier`
- **ENTONCES** la terminal imprime `BARRIER SATISFIED: All blocking roles verified`
- **Y** detalla las advertencias de tareas diferidas pendientes
- **Y** el código de salida es `0`

#### Scenario: Barrera denegada en terminal por bloqueo
- **DADO** un cambio con roles bloqueantes pendientes
- **CUANDO** se ejecuta `axiom role barrier`
- **ENTONCES** la terminal imprime `BARRIER BLOCKED` listando los roles y motivos de bloqueo
- **Y** el código de salida es `1`

# Especificación Viva: Protocolo de Absorción por Tandas desde Upstream

> **Dominio:** `upstream-absorption-protocol`  
> **Versión Canónica:** 1.0.0  
> **Estado:** Vigente  
> **Idioma:** Español (Castellano peninsular)

---

## 1. Capacidad: `upstream-absorption-protocol`

Método normativo de absorción de commits de upstream por tandas temáticas: deriva obligatoriamente la lista de ficheros de cada tanda desde el commit de origen, exige verificación sin filtrar, comprueba el inventario de no-reversión y las rutas protegidas en cada tanda, y sostiene el registro durable de absorción con su espejo en Engram.

### Requirement: Derivación obligatoria de la lista de ficheros de una tanda (REQ-20.1)

Toda tanda de absorción DEBE derivar su lista de ficheros de `git show <sha-upstream> --stat` para cada commit de upstream que la compone. El diff resultante de la tanda DEBE contrastarse contra esa derivación, y todo fichero presente en la derivación y ausente del diff DEBE llevar un motivo escrito en el registro durable de absorción. Un artefacto de planificación (`design.md`, `tasks.md`) que enumere ficheros de una tanda sin citar su derivación de `git show --stat` NO DEBE aceptarse; es motivo de rechazo de la tanda.

#### Scenario: Lista de ficheros derivada aceptada
- **DADO** una tanda que absorbe los commits de upstream `<sha1>` y `<sha2>`
- **CUANDO** se planifica su alcance de ficheros
- **ENTONCES** la lista de ficheros declarada procede de `git show <sha1> --stat` y `git show <sha2> --stat`
- **Y** cada fichero de esa derivación aparece en el diff de la tanda o lleva un motivo escrito en el registro

#### Scenario: Lista de ficheros fijada a mano rechazada
- **DADO** un artefacto de planificación que enumera ficheros de una tanda sin citar su derivación de `git show --stat`
- **CUANDO** se evalúa la tanda para su ejecución
- **ENTONCES** la tanda se rechaza
- **Y** no se ejecuta ninguna absorción sobre esa base

---

### Requirement: Verificación sin filtrar (REQ-20.2)

Toda tanda de absorción DEBE presentar como evidencia de verificación `go build ./...`, `go vet ./...`, `go test ./...` sin la bandera `-run`, y `e2e/e2e_test.sh`, todos en verde. Ninguna ejecución filtrada por patrón (`-run <patrón>`) ni la omisión de `e2e/e2e_test.sh` DEBE aceptarse como evidencia de cierre de una tanda. La cobertura no alcanzada por la suite raíz (el módulo `bench/`, que no tiene `go.work` y por tanto no entra en `go test ./...` desde la raíz) DEBE declararse explícitamente en el registro, nunca omitirse en silencio.

#### Scenario: Verificación completa aceptada
- **DADO** una tanda que completó su absorción
- **CUANDO** se registra su evidencia de verificación
- **ENTONCES** el registro cita `go build ./...`, `go vet ./...`, `go test ./...` sin `-run`, y `e2e/e2e_test.sh`, todos en verde
- **Y** declara explícitamente que `bench/` queda fuera de esa cobertura

#### Scenario: Verificación filtrada rechazada
- **DADO** una tanda cuya evidencia de verificación cita `go test -run <patrón>` o no incluye `e2e/e2e_test.sh`
- **CUANDO** se evalúa el cierre de la tanda
- **ENTONCES** la tanda no se acepta como verificada
- **Y** no se marca `absorbido` en el registro para ninguno de sus commits

---

### Requirement: Inventario de no-reversión por tanda (REQ-20.3)

El sistema DEBE tratar las entradas V1, V2, V3, V4, V5, V6 y V8 del inventario de divergencias deliberadas (renombrado de producto, detección dual de marcadores `axiom:`, agente `axiom-orchestrator` y comandos slash sin prefijo, supresión del logo de OpenCode en presets, no forzado de `theme`, persona Axiom en castellano peninsular con localización íntegra de la TUI, y la capa Hub multi-repositorio) como no-reversibles en toda tanda de absorción.

#### Scenario: Reversión de una divergencia firme rechazada
- **DADO** una tanda cuyo diff revierte, total o parcialmente, la detección dual de marcadores `axiom:` (V2) o cualquier otra entrada firme (V1, V3, V4, V5, V6 u V8)
- **CUANDO** se evalúa la tanda
- **ENTONCES** se rechaza
- **Y** se detiene el incremento hasta el último estado verde

---

### Requirement: Rutas protegidas fuera de alcance (REQ-20.4)

Ninguna tanda de absorción DEBE contener ficheros bajo `internal/hub/`, `openspec/INDEX.md` u `openspec/config.yaml`. Un diff que toque cualquiera de esas rutas DEBE rechazarse y detener el incremento hasta el último estado verde.

Ninguna tanda de absorción DEBE contener ficheros bajo `bench/`, con la única excepción de una tanda de reconciliación del corpus que alinee `bench/` con la superficie retirada de runtime sin absorber comportamiento nuevo.

#### Scenario: Diff que toca una ruta protegida rechazado
- **DADO** una tanda cuyo diff incluye un fichero bajo `internal/hub/`
- **CUANDO** se evalúa la tanda para su cierre
- **ENTONCES** se rechaza
- **Y** se revierte hasta el último estado verde

---

### Requirement: Estructura y estados del registro durable de absorción (REQ-20.5)

El sistema DEBE mantener el registro durable de absorción en `docs/upstream-absorption-ledger.md`, con una fila por cada uno de los 91 commits de upstream sin fusiones entre el ancestro común `266574b0` y el techo congelado `82a6de96` (etiqueta `v3.4.0` de upstream), medidos el 2026-09-19. Cada fila DEBE registrar como mínimo: el sha de upstream, la tanda del fork que lo absorbe, su estado (`absorbido`, `descartado-deliberadamente` o `revertido`), su evidencia de verificación, y un motivo escrito cuando el estado sea `descartado-deliberadamente` o `revertido`.

#### Scenario: Fila completa para un commit absorbido
- **DADO** el commit de upstream `18fa04fb` absorbido por la tanda F4
- **CUANDO** se consulta su fila en el registro
- **ENTONCES** el estado es `absorbido`
- **Y** cita su evidencia de verificación sin filtrar conforme a REQ-20.2

#### Scenario: Fila con motivo para un commit descartado
- **DADO** un commit de upstream cuyo contenido colisiona con una divergencia deliberada del inventario V1–V8
- **CUANDO** se registra como `descartado-deliberadamente`
- **ENTONCES** su fila incluye el motivo escrito de la exclusión

---

### Requirement: Completitud del registro al cierre, espejo en Engram e inmutabilidad de las filas (REQ-20.6)

Al cierre del incremento, el registro DEBE contener las 91 filas correspondientes a los 91 commits de upstream sin fusiones medidos el 2026-09-19 entre `266574b0` y el techo congelado `82a6de96` (etiqueta `v3.4.0`, decisión D4), cada una con un estado asignado. El sistema DEBE mantener un espejo del registro en Engram, con su medición fechada. Ninguna fila DEBE borrarse: revertir una tanda ya absorbida DEBE actualizar el estado de sus filas a `revertido`, conservando el resto de sus campos, nunca eliminar la fila.

#### Scenario: Registro completo con 91 filas al cierre
- **DADO** el cierre del incremento `inc-20-upstream-reconciliation`
- **CUANDO** se audita `docs/upstream-absorption-ledger.md`
- **ENTONCES** contiene 91 filas, cada una con estado asignado
- **Y** su espejo en Engram existe y está fechado

<!-- Especificación Viva en desarrollo — cambio 'inc-20-upstream-reconciliation' -->

# Especificación de Requerimientos: Reconciliación con Upstream e Identidad de Distribución (INC-20)

> **Incremento:** `inc-20-upstream-reconciliation`
> **Estado:** En desarrollo (fase de especificación)
> **Idioma:** Español (castellano peninsular)

## Propósito

Definir de forma rigurosa, ejecutable y verificable los requerimientos funcionales y los escenarios BDD que debe cumplir la reconciliación auditada del fork Axiom con `Gentleman-Programming/gentle-ai`: el protocolo de absorción por tandas temáticas con sus dos reglas antirrecaída (derivación obligatoria de la lista de ficheros y verificación sin filtrar), el registro durable de absorción `docs/upstream-absorption-ledger.md` y su espejo en Engram, la identidad de distribución de Axiom resuelta por clase de superficie conforme a las Decisiones D2.1 a D2.4 ya tomadas por el usuario, la cobertura de integración continua sobre el binario real `cmd/axiom`, la retirada de la gobernanza de presupuesto de intentos de ejecución absorbida de upstream preservando la concesión de autoridad de edición, el cambio de propietario declarado del libro mayor de recibos de RDD-SDD, la apertura de la investigación seleccionada de obligatoria a opcional, la elevación de Organic Driven Development (ODD) a protocolo obligatorio por defecto del orquestador, y la sustitución destructiva del paquete Go `internal/odd` por el protocolo ODD de upstream basado en instrucciones de agente, conforme a la Decisión D1 ya resuelta por el usuario.

## Alcance de esta Especificación

| # | Capacidad | Tipo | Requerimientos |
|---|---|---|---|
| 1 | `upstream-absorption-protocol` | Nueva | REQ-20.1 – REQ-20.6 |
| 2 | `axiom-distribution-identity` | Nueva | REQ-20.7 – REQ-20.14 |
| 3 | `axiom-binary-ci-coverage` | Nueva | REQ-20.15 – REQ-20.16 |
| 4 | `axiom-sdd-cli-integration` | Modificada (únicamente REQ-13.3) | REQ-13.3 |
| 5 | `rdd-sdd-receipt-consumption` | Modificada (2 requerimientos sin numerar) | «Attempt Ledger Ownership Stays With SDD»; «ReceiptRef Lives in SDD's Runtime Ledger» |
| 6 | `sdd-research` | Modificada (Propósito + 1 requerimiento sin numerar) | «Closed Capability Admission» |
| 7 | `organic-agent-trigger-rules` | Modificada (aditiva, 1 requerimiento nuevo sin numerar) | ODD como protocolo obligatorio del orquestador |
| 8 | `odd-living-document` | Modificada (destructiva) | REQ-19.1 – REQ-19.4 (RETIRADOS) |
| 9 | `odd-cli-commands` | Modificada (destructiva) | REQ-19.5 – REQ-19.8 (RETIRADOS) |
| 10 | `odd-sdd-promotion` | Modificada (destructiva) | REQ-19.9 – REQ-19.12 (RETIRADOS) |
| 11 | `odd-ui-integration` | Modificada (destructiva) | REQ-19.13 – REQ-19.15 (RETIRADOS) |

Fuera de esta especificación, por decisión de producto ya cerrada en la propuesta: la capacidad `axiom-tui-branding` (la Decisión D2.4 resuelve conservar `gentle-ai` como alias, por lo que su REQ-14.4 no cambia); la capacidad `rdd-decoupling-v3-stability` (su delta depende de una medición de solape con INC-18 que la fase F3 todavía no ha ejecutado, §4.3 de la propuesta); y las capacidades `visual-decoupling`, `workspace-topology`, `axiom-user-state-and-env`, `tui-spanish-localization`, `persona-behavior-contract` y `living-documentation`, revisadas y confirmadas sin necesidad de delta (§3.3 de la propuesta). La actualización de `docs/ROADMAP.md` y la creación de `docs/upstream-absorption-ledger.md` no son capacidades de especificación viva: la primera es mantenimiento documental y la segunda es un registro de estado vivo del repositorio fuera de `openspec/`; ambas quedan fuera del alcance de este documento.

---

## 1. Capacidad: `upstream-absorption-protocol`

Método normativo de absorción de commits de upstream por tandas temáticas: deriva obligatoriamente la lista de ficheros de cada tanda desde el commit de origen, exige verificación sin filtrar, comprueba el inventario de no-reversión y las rutas protegidas en cada tanda, y sostiene el registro durable de absorción con su espejo en Engram. Nace directamente del diagnóstico verificado de INC-18 (§4.1 de la propuesta).

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

El sistema DEBE tratar las entradas V1, V2, V3, V4, V5, V6 y V8 del inventario de divergencias deliberadas (renombrado de producto, detección dual de marcadores `axiom:`, agente `axiom-orchestrator` y comandos slash sin prefijo, supresión del logo de OpenCode en presets, no forzado de `theme`, persona Axiom en castellano peninsular con localización íntegra de la TUI, y la capa Hub multi-repositorio) como no-reversibles en toda tanda de absorción. La entrada V7 (ODD como paquete Go) quedó resuelta por la Decisión D1 hacia la sustitución completa por el ODD de upstream: únicamente la fase F6 está autorizada a retirarla, y solo mediante los deltas de las capacidades `odd-living-document`, `odd-cli-commands`, `odd-sdd-promotion` y `odd-ui-integration` de este mismo documento; toda tanda distinta de F6 DEBE seguir tratando V7 como no-reversible mientras F6 no se haya ejecutado.

#### Scenario: Reversión de una divergencia firme rechazada

- **DADO** una tanda cuyo diff revierte, total o parcialmente, la detección dual de marcadores `axiom:` (V2) o cualquier otra entrada firme (V1, V3, V4, V5, V6 u V8)
- **CUANDO** se evalúa la tanda
- **ENTONCES** se rechaza
- **Y** se detiene el incremento hasta el último estado verde

#### Scenario: F6 retira V7 con autorización explícita

- **DADO** la fase F6 y la Decisión D1 resuelta hacia la sustitución completa del ODD
- **CUANDO** F6 retira el paquete Go `internal/odd`, su CLI y sus superficies TUI y Dashboard Web
- **ENTONCES** esa retirada no se trata como una violación del inventario de no-reversión
- **Y** toda tanda distinta de F6 sigue tratando V7 como no-reversible mientras F6 no se haya ejecutado

---

### Requirement: Rutas protegidas fuera de alcance (REQ-20.4)

Ninguna tanda de absorción DEBE contener ficheros bajo `bench/`, `internal/hub/`, `openspec/INDEX.md` u `openspec/config.yaml`. Un diff que toque cualquiera de esas rutas DEBE rechazarse y detener el incremento hasta el último estado verde.

#### Scenario: Diff que toca una ruta protegida rechazado

- **DADO** una tanda cuyo diff incluye un fichero bajo `internal/hub/`
- **CUANDO** se evalúa la tanda para su cierre
- **ENTONCES** se rechaza
- **Y** se revierte hasta el último estado verde

#### Scenario: bench/ fuera de la cobertura no bloquea el cierre

- **DADO** que `bench/` es un módulo Go independiente sin `go.work`
- **CUANDO** una tanda cierra su verificación sin tocar ningún fichero de `bench/`
- **ENTONCES** la ausencia de cobertura de `bench/` se declara explícitamente
- **Y** no impide marcar la tanda como `absorbido`

---

### Requirement: Estructura y estados del registro durable de absorción (REQ-20.5)

El sistema DEBE mantener el registro durable de absorción en `docs/upstream-absorption-ledger.md`, con una fila por cada uno de los 91 commits de upstream sin fusiones entre el ancestro común `266574b0` y el techo congelado `82a6de96` (etiqueta `v3.4.0` de upstream), medidos el 2026-09-19. La cifra de 55 medida el 2026-09-18 quedó superada por la decisión D4, que congela el universo en `v3.4.0` y no lo re-mide. Cada fila DEBE registrar como mínimo: el sha de upstream, la tanda del fork que lo absorbe, su estado (`absorbido`, `descartado-deliberadamente` o `revertido`), su evidencia de verificación, y un motivo escrito cuando el estado sea `descartado-deliberadamente` o `revertido`.

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

#### Scenario: Reversión actualiza el estado sin borrar la fila

- **DADO** una tanda absorbida y luego revertida mediante `git revert` de su commit de fusión
- **CUANDO** se actualiza el registro tras la reversión
- **ENTONCES** las filas de esa tanda pasan a estado `revertido`
- **Y** ninguna fila desaparece del registro

---

## 2. Capacidad: `axiom-distribution-identity`

Contrato de nombre publicado por clase de superficie: instalador y tap, compuertas de release, workflows de CI, shim de `crosslane`, namespace de protocolo en `contracts/**`, ruta de módulo Go y nombres de servicio de telemetría de despliegue. Distingue interoperabilidad (lo que lee una máquina, no renombrable sin romper consumidores) de identidad (lo que lee un humano, renombrable), y resuelve cada superficie conforme a las Decisiones D2.1 a D2.4 ya tomadas por el usuario.

### Requirement: Taxonomía de interoperabilidad vs. identidad (REQ-20.7)

El sistema DEBE clasificar cada superficie de nombre publicado en exactamente una de dos categorías: interoperabilidad (contrato de cable consumido por otro software, cuyo renombrado rompe a consumidores) o identidad (superficie leída por una persona, renombrable sin romper ningún contrato). El namespace de protocolo bajo `contracts/**` (los valores `$id` y los prefijos de contrato como `gentle-ai.review-integration/v2` y `gentle-ai.sdd-status/v2`) y los marcadores de contenido gestionado que un componente Go analiza de forma literal (por ejemplo `<!-- gentle-ai:sdd-session-preflight -->` en `internal/components/sdd/session_preflight.go`) DEBEN clasificarse como interoperabilidad. El instalador, el tap de Homebrew, las compuertas de release, el nombre de binario citado en la documentación y los nombres de servicio de despliegue DEBEN clasificarse como identidad.

#### Scenario: El namespace de contracts/** se clasifica como interoperabilidad

- **DADO** el fichero `contracts/review-integration/v2/schemas/status.schema.json` con `"$id": "https://gentle-ai.dev/contracts/review-integration/v2/schemas/status.schema.json"` y `"contract": "gentle-ai.review-integration/v2"`
- **CUANDO** se clasifica esa superficie
- **ENTONCES** se clasifica como interoperabilidad
- **Y** ninguna tanda de identidad la modifica

#### Scenario: El instalador se clasifica como identidad

- **DADO** `scripts/install.sh` con `GITHUB_OWNER`, `GITHUB_REPO` y `BINARY_NAME`
- **CUANDO** se clasifica esa superficie
- **ENTONCES** se clasifica como identidad
- **Y** su resolución sigue la Decisión D2.1

---

### Requirement: Instalador, tap y compuertas de release condicionados a la publicación de releases propios (REQ-20.8)

El instalador (`scripts/install.sh`, `scripts/install.ps1`), el tap de Homebrew y las compuertas de release (`scripts/verify-release-assets.sh`, `scripts/release-preflight.sh`, `scripts/promote-stable-preflight.sh`) DEBEN apuntar a la identidad de distribución de Axiom, conforme a la Decisión D2.1. El sistema NO DEBE repuntar ninguna de esas superficies hacia la distribución propia de Axiom hasta que existan artefactos de release propios publicados bajo esa identidad; repuntarlas antes dejaría el instalador sin nada que instalar.

#### Scenario: El instalador no se repunta sin releases propios

- **DADO** que Axiom aún no ha publicado ningún artefacto de release bajo su propia identidad de distribución
- **CUANDO** se evalúa si repuntar `scripts/install.sh` hacia esa identidad
- **ENTONCES** el repunte no se ejecuta todavía
- **Y** el instalador sigue apuntando a una distribución que efectivamente instala un binario

#### Scenario: El instalador se repunta tras publicar releases propios

- **DADO** que Axiom publicó su primer artefacto de release firmado bajo su propia identidad de distribución
- **CUANDO** se ejecuta la tanda F0
- **ENTONCES** `scripts/install.sh`, `scripts/install.ps1`, el tap de Homebrew y las tres compuertas de release citan esa identidad
- **Y** el instalador resultante instala el binario `axiom`, no el de upstream

---

### Requirement: Ruta de módulo Go migrada a /v3 (REQ-20.9)

La ruta de módulo Go DEBE migrar de `github.com/gentleman-programming/gentle-ai/v2` al destino que sigue a upstream, `github.com/gentleman-programming/gentle-ai/v3` (commit de origen `2594581e`), conforme a la Decisión D2.2. La migración DEBE ser una única reescritura mecánica aplicada a todos los ficheros que citan la ruta anterior; el sistema NO DEBE aplicar una segunda reescritura masiva de rutas de importación dentro de este incremento.

#### Scenario: Migración mecánica completa de la ruta de módulo

- **DADO** el módulo declarado como `github.com/gentleman-programming/gentle-ai/v2` en `go.mod`
- **CUANDO** se ejecuta la fase F1
- **ENTONCES** `go.mod` y todo fichero que importaba la ruta `/v2` pasan a citar `/v3`
- **Y** `go build ./...` compila en verde tras la migración

#### Scenario: Ninguna segunda reescritura masiva dentro del incremento

- **DADO** que la ruta de módulo ya migró a `/v3` en la fase F1
- **CUANDO** se ejecutan las fases F2 a F7
- **ENTONCES** ninguna de ellas vuelve a reescribir la ruta de importación de forma masiva

---

### Requirement: Namespace de protocolo en contracts/** sin cambios (REQ-20.10)

El sistema NO DEBE modificar el namespace de protocolo bajo `contracts/**`: los valores `$id`, los prefijos de contrato (`gentle-ai.review-integration/v2`, `gentle-ai.sdd-status/v2` y equivalentes) y los esquemas asociados DEBEN permanecer byte a byte idénticos, conforme a la Decisión D2.3.

#### Scenario: contracts/** permanece sin cambios tras el incremento

- **DADO** el estado de `contracts/**/*.schema.json` antes de iniciar la reconciliación
- **CUANDO** se cierra el incremento
- **ENTONCES** cada fichero bajo `contracts/**` es byte a byte idéntico a su estado previo

#### Scenario: Un consumidor compatible con upstream sigue interoperando

- **DADO** un consumidor externo que valida contra `gentle-ai.review-integration/v2`
- **CUANDO** Axiom publica su superficie de identidad ya resuelta por D2.1, D2.2 y D2.4
- **ENTONCES** ese consumidor sigue validando sin cambios contra `contracts/**`

---

### Requirement: Pasarela gentle-ai y shim de crosslane conservados (REQ-20.11)

El sistema DEBE conservar `cmd/gentle-ai` como pasarela de deprecación hacia `axiom`, y DEBE conservar sin cambios de comportamiento el shim ejecutable `gentle-ai` en `$PATH` que generan `scripts/crosslane/battery.go` y `scripts/crosslane/host.go`, incluido su análisis por `words[0]`, conforme a la Decisión D2.4.

#### Scenario: cmd/gentle-ai sigue emitiendo su aviso de deprecación

- **DADO** el binario compilado desde `cmd/gentle-ai`
- **CUANDO** se invoca con cualquier argumento
- **ENTONCES** emite el aviso de deprecación hacia `axiom` en la salida de error estándar
- **Y** delega la ejecución al mismo comportamiento que `internal/app.RunArgs`

#### Scenario: El shim de crosslane sigue resolviendo por el nombre gentle-ai

- **DADO** una batería de pruebas de revisor que invoca el shim ejecutable `gentle-ai` en `$PATH`
- **CUANDO** el shim analiza `words[0]`
- **ENTONCES** sigue reconociendo `gentle-ai` como nombre válido sin cambios en `battery.go` ni en `host.go`

---

### Requirement: Nombres de servicio de telemetría de despliegue (REQ-20.12)

Los nombres de servicio de despliegue de telemetría (`deploy/telemetry/gentle-telemetry.service`, `deploy/telemetry/gentle-telemetry-backup.service` y los paneles de Grafana asociados) DEBEN resolverse hacia la identidad de distribución de Axiom, conforme al criterio general de la Decisión D2 («renombra lo que lee un humano, conserva lo que lee una máquina»). Esta superficie es de despliegue de servidor y es distinta del estado de usuario `~/.axiom` / `~/.gentle-ai`, que gobierna la capacidad `axiom-user-state-and-env` y que este incremento no toca.

#### Scenario: Los nombres de servicio de telemetría se renombran

- **DADO** `deploy/telemetry/gentle-telemetry.service` y `deploy/telemetry/gentle-telemetry-backup.service`
- **CUANDO** se ejecuta la tanda F0
- **ENTONCES** los ficheros de unidad y los paneles de Grafana asociados citan la identidad de distribución de Axiom

#### Scenario: El estado de usuario permanece fuera de esta capacidad

- **DADO** la precedencia de `~/.axiom` sobre `~/.gentle-ai` para el estado de usuario
- **CUANDO** se resuelve esta capacidad
- **ENTONCES** ningún requerimiento de `axiom-distribution-identity` impone cambios sobre esa precedencia

---

**Excepción explícita a la exclusión anterior: la raíz de respaldos.** REQ-20.12 excluye el estado de usuario opaco (`state.json`, precedencia `AXIOM_*`) porque lo gobierna `axiom-user-state-and-env` sin necesidad de delta. La raíz de respaldos es distinta: `internal/backup/manifest.go:170-186` la declara contrato publicado de forma explícita en sus propios comentarios (`backupRoot` → `~/.axiom/backups`, canónica; `legacyBackupRoot` → `~/.gentle-ai/backups`, heredada), y `cmd/axiom/main.go:141` la nombra en un mensaje al usuario. Es, por tanto, identidad renombrable leída por un humano conforme a la taxonomía de REQ-20.7, no estado opaco. Medido: el paquete declara la raíz canónica y sus lectores (`internal/backup/manifest.go:176`, `internal/app/app.go:1098-1099`) la consultan correctamente con respaldo a la heredada, pero los escritores de producción (`internal/cli/sync.go:509`, `internal/cli/run.go:709`, `internal/update/upgrade/executor.go:488,514`, `internal/components/uninstall/service.go:178`) construyen la ruta con un literal `.gentle-ai/backups` en vez de invocar la función exportada del paquete, y `internal/dashboard/service.go:1190` hace lo mismo con un literal `.axiom/backups` que hoy coincide con el valor canónico solo por coincidencia. El resultado medido es que todo respaldo nuevo se escribe bajo la grafía retirada mientras el mensaje al usuario nombra la vigente.

### Requirement: Raíz de respaldos resuelta exclusivamente a través de internal/backup (REQ-20.13)

Todo escritor de producción que cree o localice el directorio raíz de respaldos DEBE resolverlo invocando la función de resolución canónica exportada de `internal/backup` (`backup.BackupRootFn()` o su equivalente), y NO DEBE construir esa ruta mediante un literal `filepath.Join` propio. `internal/cli/sync.go`, `internal/cli/run.go`, `internal/update/upgrade/executor.go` e `internal/components/uninstall/service.go` DEBEN dejar de citar `.gentle-ai/backups` como literal; `internal/dashboard/service.go` DEBE dejar de citar `.axiom/backups` como literal, aunque su valor ya coincida con el canónico. Un respaldo escrito después de esta absorción DEBE ser legible por `ListBackups` sin depender de su rama de compatibilidad con la grafía heredada. Los respaldos ya existentes bajo `~/.gentle-ai/backups` DEBEN seguir siendo legibles por `ListBackups` sin migración obligatoria. El mensaje de ausencia de respaldos de `axiom backup` (`cmd/axiom/main.go:141`) DEBE nombrar la raíz donde los escritores de producción escriben realmente.

#### Scenario: Un escritor de producción resuelve la raíz a través del paquete

- **DADO** cualquiera de `internal/cli/sync.go`, `internal/cli/run.go`, `internal/update/upgrade/executor.go`, `internal/components/uninstall/service.go` o `internal/dashboard/service.go`
- **CUANDO** necesita el directorio raíz de respaldos
- **ENTONCES** lo obtiene invocando la función de resolución canónica de `internal/backup`
- **Y** ningún literal `filepath.Join(homeDir, ".gentle-ai", "backups")` ni `filepath.Join(home, ".axiom", "backups")` permanece en esos ficheros

#### Scenario: Un respaldo nuevo es legible sin depender de la rama heredada

- **DADO** un respaldo creado tras absorber esta tanda
- **CUANDO** `ListBackups` enumera los respaldos disponibles
- **ENTONCES** ese respaldo aparece resuelto desde la raíz canónica `.axiom/backups`
- **Y** su descubrimiento no depende de que `ListBackups` recurra a la raíz heredada

#### Scenario: Los respaldos heredados siguen siendo legibles

- **DADO** un respaldo existente bajo `~/.gentle-ai/backups` creado antes de esta absorción
- **CUANDO** `ListBackups` enumera los respaldos disponibles tras la absorción
- **ENTONCES** ese respaldo heredado sigue apareciendo en el listado
- **Y** ningún dato de respaldo existente se pierde ni requiere migración manual

#### Scenario: El mensaje de ausencia de respaldos nombra la raíz real de escritura

- **DADO** que ningún respaldo existe todavía
- **CUANDO** el usuario ejecuta `axiom backup`
- **ENTONCES** el mensaje de ausencia nombra la misma raíz que usan los escritores de producción tras esta absorción
- **Y** esa raíz coincide con la que `ListBackups` consulta en primer lugar

---

### Requirement: Guarda ejecutable contra rutas literales de raíz de respaldos (REQ-20.14)

El sistema DEBE mantener una prueba automatizada que analice los ficheros de producción de `internal/cli`, `internal/update/upgrade`, `internal/components/uninstall` e `internal/dashboard`, y que falle si alguno construye la raíz de respaldos mediante un literal `.gentle-ai/backups` o `.axiom/backups` en lugar de invocar la función de resolución canónica de `internal/backup`, siguiendo el mismo patrón de guarda por análisis estático ya establecido en este repositorio (`internal/reviewtransaction/compact_receipt_gate_removal_guard_test.go`). Esta guarda DEBE ejecutarse como parte de `go test ./...` sin bandera `-run`, conforme a REQ-20.2.

#### Scenario: La guarda falla si reaparece una ruta literal

- **DADO** un cambio futuro que reintroduce `filepath.Join(homeDir, ".gentle-ai", "backups")` en un fichero de producción de los paquetes vigilados
- **CUANDO** se ejecuta `go test ./...`
- **ENTONCES** la guarda falla y nombra el fichero y la línea del literal reintroducido

#### Scenario: La guarda pasa cuando todos los escritores usan la función canónica

- **DADO** que todos los escritores de producción vigilados resuelven la raíz a través de `internal/backup`
- **CUANDO** se ejecuta `go test ./...`
- **ENTONCES** la guarda pasa sin señalar ningún literal

**Enmienda (Fase 1 de F0.a, 2026-09-19): la guarda no puede limitarse a `filepath.Join`.** La redacción original nombraba ese patrón, y la implementación de la Fase 1 lo siguió al pie de la letra. Al ejecutarla en rojo destapó ocho sitios en seis ficheros —y **uno más que no vio**:

```go
// internal/cli/restore.go:225
return homeDir + "/.gentle-ai/backups"
```

`backupRootDir` construye la raíz heredada por **concatenación de cadenas**, con separador codificado a mano. Ninguna auditoría basada en `filepath.Join` la detecta, ni la guarda ni la tabla de ficheros del diseño. Y solo lee la raíz heredada: en cuanto los escritores migren, `axiom restore --list` y `RunRestore` dejarán de ver los respaldos nuevos. Es una regresión funcional silenciosa, no una omisión cosmética.

Por tanto, la obligación de este requerimiento se amplía: la guarda DEBE detectar **cualquier** forma sintáctica de construir una raíz de estado de usuario fuera del paquete que la posee —`filepath.Join`, concatenación con `+`, `fmt.Sprintf`, literal completo— y no solo la invocación de `filepath.Join`. El primer escenario se lee en adelante con esa extensión.

`internal/cli` ya figura entre los paquetes vigilados, así que el alcance de la guarda no cambia: cambia lo que sabe reconocer dentro de él.

#### Scenario: La guarda detecta la concatenación de cadenas

- **DADO** un fichero de producción vigilado que construye la raíz como `homeDir + "/.gentle-ai/backups"` en lugar de invocar la función canónica
- **CUANDO** se ejecuta `go test ./...`
- **ENTONCES** la guarda falla y nombra ese fichero y esa línea, igual que haría con un `filepath.Join`

---

## 3. Capacidad: `axiom-binary-ci-coverage`

El pipeline de CI DEBE construir y ejercitar `cmd/axiom`, incluida su superficie exclusiva, con una ventana informativa acotada y registrada cuando el paso destape fallos preexistentes fuera de esa superficie.

### Requirement: Construcción y ejercicio bloqueante de la superficie exclusiva de axiom (REQ-20.15)

El flujo de trabajo de CI DEBE construir `cmd/axiom` y ejercitar de forma bloqueante, para al menos los comandos que toca el contrato de nombre resuelto por `axiom-distribution-identity`, su superficie exclusiva: `init`, `change`, `project`, `workspace`, `handoff`, `role`, `skill`, `semantic`, `archive`, `odd` y `ui`. Esta obligación es adicional a la construcción y el ejercicio ya existentes de `cmd/axiom` en el job `unit-tests` a través del corpus de journeys de `bench/` (paso «Run benchmark evidence»), que cubren la superficie de revisión y ciclo de vida SDD compartida con upstream pero no ejercitan ningún comando de la superficie exclusiva de Axiom.

#### Scenario: Humo bloqueante sobre la superficie exclusiva

- **DADO** el binario `cmd/axiom` recién construido en CI
- **CUANDO** el paso de cobertura se ejecuta sobre un repositorio de prueba
- **ENTONCES** ejercita al menos `axiom init`, `axiom change create`, `axiom workspace validate`, `axiom handoff show`, `axiom role list`, `axiom skill list`, `axiom semantic status`, `axiom archive list`, `axiom odd status` y el arranque de `axiom ui`
- **Y** un fallo en cualquiera de ellos bloquea la fusión

#### Scenario: La cobertura de journeys compartidas no sustituye a la exclusiva

- **DADO** que el paso «Run benchmark evidence» ya construye y ejercita `cmd/axiom` sobre journeys heredadas de revisión y ciclo de vida SDD (por ejemplo `j51`, `j110`, la serie `tr01`–`tr11`)
- **CUANDO** se evalúa si esa cobertura satisface esta capacidad
- **ENTONCES** no la satisface por sí sola, porque ninguna de esas journeys invoca la superficie exclusiva de Axiom
- **Y** el paso bloqueante de esta capacidad se añade a esa cobertura existente, no la sustituye

---

### Requirement: Ventana informativa acotada y registrada para el resto de la superficie roja (REQ-20.16)

Si el paso de cobertura de `cmd/axiom` destapa fallos preexistentes fuera de la superficie que bloquea el contrato de nombre, el sistema DEBE mantener esa parte como informativa (no bloqueante) únicamente durante una ventana acotada. Esa ventana DEBE registrar por escrito el inventario de la superficie roja y DEBE nombrar el incremento sucesor que la asume. El sistema NO DEBE dejar una ventana informativa sin fecha ni sucesor nombrado.

#### Scenario: Ventana informativa con inventario e incremento sucesor

- **DADO** que el paso de cobertura destapa fallos en una superficie de `cmd/axiom` no cubierta por el contrato de nombre
- **CUANDO** se registra esa parte como informativa
- **ENTONCES** el registro incluye el inventario escrito de la superficie roja y el nombre del incremento sucesor

#### Scenario: Ventana sin sucesor nombrado rechazada

- **DADO** un paso de CI que marca parte de la superficie de `cmd/axiom` como informativa sin nombrar un incremento sucesor
- **CUANDO** se audita esa configuración
- **ENTONCES** se rechaza como incompleta
- **Y** no se acepta como cierre de esta capacidad

---

## 4. Capacidad Modificada: `axiom-sdd-cli-integration`

Esta capacidad ya existe en `openspec/specs/axiom-sdd-cli-integration/spec.md`. El bloque siguiente sustituye por completo el requerimiento REQ-13.3 en esa especificación viva, encabezado y escenario incluidos. Los requerimientos REQ-13.1 (`axiom sdd status`) y REQ-13.2 (`axiom sdd continue`), de la misma capacidad, y REQ-13.4 (capacidad `orchestrator-prompt-call-alignment`, mismo fichero), no se modifican y DEBEN conservarse sin cambios al fusionar este delta.

### Requirement: Subcomando axiom sdd attempt tras la retirada de la gobernanza de presupuesto (REQ-13.3)

El binario `axiom` NO DEBE soportar las operaciones `acquire` y `settle` del libro mayor de intentos de ejecución bajo `axiom sdd attempt`, siguiendo la retirada de la gobernanza de *attempts* absorbida de upstream (commit `18fa04fb`). El binario `axiom` DEBE conservar sin cambio de comportamiento la operación `axiom sdd attempt grant`, que registra la autoridad de edición por raíz (`--root`) para el cambio activo.

(Previamente: el binario `axiom` DEBÍA soportar las operaciones `acquire` y `settle` del libro mayor de intentos de ejecución, `axiom sdd attempt acquire` y `axiom sdd attempt settle`.)

#### Scenario: acquire y settle dejan de estar disponibles

- **DADO** un cambio en fase `sdd-apply` tras absorber la tanda F4
- **CUANDO** se invoca `axiom sdd attempt acquire --change <cambio> ...` o `axiom sdd attempt settle --token <token> ...`
- **ENTONCES** el binario rechaza la operación como no reconocida
- **Y** no retorna ningún token de sesión ni registra ningún resultado de presupuesto

#### Scenario: grant sigue emitiendo autoridad de edición sin cambios

- **DADO** un cambio activo con raíces de edición pendientes de autorizar
- **CUANDO** se invoca `axiom sdd attempt grant --root <ruta> --change-instance <token> --request-id <id> --actor <actor> --reason <motivo>`
- **ENTONCES** el sistema registra la autoridad de edición para esa raíz
- **Y** el comportamiento de `grant` no cambia respecto al contrato vigente antes de esta absorción

---

## 5. Capacidad Modificada: `rdd-sdd-receipt-consumption`

Esta capacidad ya existe en `openspec/specs/rdd-sdd-receipt-consumption/spec.md`. Los dos bloques siguientes sustituyen por completo los requerimientos «Attempt Ledger Ownership Stays With SDD (Maintainer-Confirmed, 2026-08-02)» y «ReceiptRef Lives in SDD's Runtime Ledger, Not a New Artifact» en esa especificación viva, encabezado y escenarios incluidos. Los requerimientos «ReceiptRef-Only Persistence», «No Re-Derived Review-Lifecycle Meaning» y «Legacy `reviewGate` v1 Field Compatibility», de la misma capacidad, no se modifican y DEBEN conservarse sin cambios al fusionar este delta.

### Requirement: La propiedad del libro mayor de intentos permanece en SDD, ahora en solitario (antes: Decisión 9, confirmada por mantenedor el 2026-08-02)

SDD conserva la propiedad exclusiva de sus propios intentos de unidad de trabajo en `runtime_ledger.go`, porque el encadenamiento de `previous_revision`, el `expected_revision` de tipo CAS y la identidad de repetición de `request_digest` ya satisfacen las propiedades de un registro acumulativo durable. Tras la absorción de la retirada de RDD del ciclo de vida SDD (commit de upstream `e0774e05`), esta propiedad deja de coordinarse con RDD: RDD ya no participa del ciclo de vida SDD y no reclama ninguna propiedad sobre el libro mayor de intentos. `RuntimeObjective` DEBE seguir siendo el único propietario nombrado del alcance de unidad de trabajo a través de `runtime_ledger.go` y `runtime_compact.go`; los campos de unidad de trabajo de `CompactAcquireRequest` DEBEN seguir colapsados en `BeginAttemptRequest`.

(Previamente: la propiedad se declaraba coordinada con RDD —«RDD owns only the receipt»— bajo la Decisión 9 confirmada por mantenedor el 2026-08-02.)

#### Scenario: Los intentos permanecen en el libro mayor de runtime de SDD sin ninguna contraparte en RDD

- **DADO** que la retirada de RDD del ciclo de vida SDD ya está absorbida
- **CUANDO** un intento de unidad de trabajo se completa
- **ENTONCES** se anexa a la cadena CAS de `runtime_ledger.go`
- **Y** ningún almacén de autoridad de RDD reclama ni conserva un registro de intento correspondiente, porque RDD ya no forma parte del ciclo de vida SDD

#### Scenario: Un único propietario nombrado para compactación y libro mayor

- **DADO** que `runtime_ledger.go` y `runtime_compact.go` tocan ambos el alcance de unidad de trabajo
- **CUANDO** se documenta la propiedad
- **ENTONCES** se registra exactamente un componente/propietario nombrado para ambos ficheros
- **Y** no existe ninguna reclamación de propiedad competidora, ni de RDD ni de ningún otro componente

---

### Requirement: ReceiptRef pasa a ser un campo histórico de solo lectura en el libro mayor de runtime de SDD

Tras la absorción de la retirada de RDD del ciclo de vida SDD (commit de upstream `e0774e05`), SDD NO DEBE escribir un nuevo valor de `ReceiptRef` en ningún intento de su libro mayor de runtime, porque ya no existe una relación activa con RDD que lo produzca. SDD DEBE seguir leyendo, de forma exclusivamente informativa y sin re-derivar significado de ciclo de vida de revisión, cualquier `ReceiptRef` ya persistido en un intento anterior a esta absorción. SDD NO DEBE introducir un fichero de artefacto OpenSpec nuevo y autónomo para sustituir ese campo.

(Previamente: SDD DEBÍA almacenar el `ReceiptRef` de todo resultado de revisión finalizado como campo del registro de intento existente en su libro mayor de runtime, sin introducir un artefacto nuevo.)

#### Scenario: Un intento anterior a la absorción conserva su ReceiptRef como dato histórico

- **DADO** un intento de unidad de trabajo cuyo registro en el libro mayor de runtime ya contenía un `ReceiptRef` antes de absorber la retirada de RDD
- **CUANDO** SDD carga el estado de ese cambio
- **ENTONCES** el campo `ReceiptRef` permanece legible como dato histórico
- **Y** SDD no re-deriva de él ningún significado de ciclo de vida de revisión

#### Scenario: Ningún intento nuevo escribe un ReceiptRef tras la absorción

- **DADO** que la retirada de RDD del ciclo de vida SDD ya está absorbida
- **CUANDO** un intento de unidad de trabajo se completa
- **ENTONCES** su registro en el libro mayor de runtime no escribe ningún valor nuevo de `ReceiptRef`
- **Y** no se introduce ningún fichero de artefacto OpenSpec nuevo para sustituir ese campo

---

## 6. Capacidad Modificada: `sdd-research`

Esta capacidad ya existe en `openspec/specs/sdd-research/spec.md`. El bloque siguiente sustituye por completo el requerimiento «Closed Capability Admission» en esa especificación viva, encabezado y escenarios incluidos, y además sustituye la declaración única de `## Purpose` del fichero completo. Los requerimientos «Auditable Evidence Integrity» y «Hybrid Completion and Recovery», de la misma capacidad, no se modifican y DEBEN conservarse sin cambios al fusionar este delta.

**Propósito actualizado** (sustituye por completo la línea única de `## Purpose` en el fichero):

> La investigación seleccionada es opcional. Cuando el orquestador decide invocarla, se ejecuta bajo admisión de capacidad verificada; su ausencia no bloquea la propuesta por sí sola.

(Previamente: `## Purpose` declaraba únicamente «Selected research is required».)

### Requirement: Admisión abierta de capacidad para investigación opcional (antes: Closed Capability Admission)

El carril de investigación seleccionada es opcional, no obligatorio, conforme a la sustitución de la admisión absorbida de upstream (commit `ba3ed690`). El carril DEBE seguir aceptando únicamente `gentle-ai.sdd-research-capability/v1` y las clases `documentation` y `open-web`; la admisión DEBE seguir verificando declaración y concesión exacta. Bash, MCP genérico y herramientas heredadas sin nombrar DEBEN seguir denegándose. La ausencia de una invocación de investigación seleccionada para un cambio dado NO DEBE bloquear la propuesta ni ningún otro artefacto SDD por sí sola.

(Previamente: el carril debía aceptar únicamente `gentle-ai.sdd-research-capability/v1` y las clases `documentation` y `open-web` bajo un modelo de admisión cerrada y obligatoria, sin ninguna cláusula sobre la ausencia de investigación.)

#### Scenario: Solicitud admitida cuando se invoca la investigación

- **DADO** que el orquestador decide invocar investigación seleccionada para un cambio
- **Y** una clase conocida y seleccionada tiene su concesión exacta declarada
- **CUANDO** se ejecuta la admisión
- **ENTONCES** registra la concesión y permite la investigación

#### Scenario: Capacidad desconocida o denegada sigue bloqueando esa invocación

- **DADO** una solicitud con clase/versión desconocida o herramienta no verificable
- **CUANDO** se ejecuta la admisión
- **ENTONCES** registra la denegación, no emite ninguna afirmación y bloquea esa invocación
- **Y** ese bloqueo no impide que el cambio continúe por un carril de planificación que no dependa de esa investigación

#### Scenario: Ausencia de investigación seleccionada no bloquea la propuesta

- **DADO** un cambio para el que el orquestador no invocó investigación seleccionada
- **CUANDO** se redacta la propuesta del cambio
- **ENTONCES** la ausencia de esa investigación no bloquea la propuesta por sí sola
- **Y** cualquier bloqueo declarado en `sdd-propose` proviene de una razón distinta a la ausencia de investigación seleccionada

---

## 7. Capacidad Modificada: `organic-agent-trigger-rules`

Esta capacidad ya existe en `openspec/specs/organic-agent-trigger-rules/spec.md`. El bloque siguiente es un requerimiento `ADDED`: se añade a los ya existentes sin modificar ninguno de ellos. En particular, el requerimiento «SDD remains optional» no cambia; el requerimiento nuevo declara explícitamente cómo se relaciona con él.

## ADDED Requirements

### Requirement: Organic Driven Development como protocolo obligatorio por defecto del orquestador

Las instrucciones de enrutamiento gestionadas DEBEN proyectar el protocolo Organic Driven Development (ODD) como el flujo de trabajo predefinido del orquestador para toda solicitud, ejecutado en primer lugar y sin que el usuario deba solicitarlo explícitamente ni preguntar por planificación o seguimiento de tareas, conforme a la absorción de upstream (commits `70c774f8`, `1b202d77`, `cfc415ce`). Esta proyección DEBE incluir, como mínimo, el commit por unidad de trabajo, el TDD configurado y la continuidad de *feature* al reanudar. Las instrucciones de enrutamiento hacia SDD (`direct_inline`, `delegated-direct`, `optional-SDD`) NO DEBEN sustituir, preceder ni posponer la proyección de ODD; SDD solo se activa tras una selección explícita del usuario o la aceptación de una propuesta SDD, exactamente como ya rige el requerimiento «SDD remains optional» de esta misma capacidad.

#### Scenario: ODD se proyecta antes de cualquier decisión de enrutamiento

- **DADO** un adaptador registrado por el proveedor con un manifiesto de capacidad válido
- **CUANDO** se renderizan sus instrucciones de enrutamiento gestionadas
- **ENTONCES** el protocolo ODD queda proyectado como flujo predefinido, ejecutado antes de evaluar `direct_inline`, `delegated-direct` u `optional-SDD`
- **Y** ninguna de esas tres facetas de enrutamiento se presenta como sustituto de la proyección de ODD

#### Scenario: SDD sigue exigiendo selección explícita incluso con ODD por defecto

- **DADO** que ODD se proyecta como protocolo predefinido del orquestador
- **CUANDO** el trabajo no ha recibido una solicitud explícita de SDD ni la aceptación de una propuesta SDD
- **ENTONCES** el enrutamiento hacia `optional-SDD` permanece sin activarse
- **Y** el trabajo continúa bajo la proyección de ODD u otra ruta de implementación directa

---

## 8. Capacidad Modificada (Retirada Destructiva): `odd-living-document`

**Aviso de delta destructivo** (conforme a `rules.archive` de `openspec/config.yaml`): la Decisión D1 resuelve la sustitución completa del ODD del fork por el ODD de upstream, sin conservar la capa Go. Los cuatro requerimientos siguientes, archivados el 2026-09-18 como parte de INC-19, se retiran en su totalidad. Al retirarlos, esta capacidad queda sin ningún requerimiento vivo; corresponde a la fase de archivado decidir si `openspec/specs/odd-living-document/spec.md` se elimina por completo o se conserva como fichero vacío con nota de retirada — esta especificación no lo prescribe, solo que ninguno de sus cuatro requerimientos sobrevive.

## REMOVED Requirements

### Requirement: Estructura Canónica del Documento Vivo ODD (REQ-19.1)

(Motivo: la Decisión D1 retira por completo el paquete Go `internal/odd` —incluidos `document.go`, `render.go`, `template.go` y `store.go`, que implementaban esta estructura de doce secciones— y lo sustituye por el protocolo ODD de upstream, basado en instrucciones de agente inyectadas en `internal/components/agentguidance/routing.go`.)
(Migración: los documentos `odd/tasks/<feature>.md` y sus espejos Engram `odd/<feature>/tasks` ya existentes no se modifican ni se eliminan; son datos, no código. El contrato de forma del documento continúa como convención de facto sostenida por instrucciones de agente, fuera del alcance de `openspec/specs/`; no existe una capacidad Go sucesora verificable por test para esta estructura.)

### Requirement: Identidad Estable de Feature y Ubicación del Fichero (REQ-19.2)

(Motivo: la validación de kebab-case y el rechazo de colisión de nombre de *feature* los implementaba `internal/odd/name.go`, retirado junto con el resto del paquete por la Decisión D1.)
(Migración: ninguna. La resolución de nombre de *feature* pasa a ser una convención de agente sin verificación Go.)

### Requirement: Identificadores Estables de Tarea y Derivación de Progreso (REQ-19.3)

(Motivo: la derivación de progreso mediante `multirole.CountTasks` sobre el checklist accionable estaba implementada en `internal/odd/parse.go` y `internal/odd/document.go`, retirados por la Decisión D1.)
(Migración: el protocolo ODD de upstream reporta continuidad de *feature*, TDD configurado y commit por unidad de trabajo como doctrina de agente, sin un mecanismo Go equivalente y verificable por test en este fork.)

### Requirement: Contrato del Espejo de Recuperación en Engram y Política de Divergencia (REQ-19.4)

(Motivo: la comparación fichero-vs-espejo y la política de autoridad las implementaba `internal/odd/mirror.go`, retirado por la Decisión D1.)
(Migración: el espejo Engram `odd/<feature>/tasks` sigue siendo la convención que el agente mantiene por MCP, ahora sin ninguna verificación del lado Go; no hay comprobación automática de divergencia equivalente a `--check-mirror` tras esta retirada.)

---

## 9. Capacidad Modificada (Retirada Destructiva): `odd-cli-commands`

**Aviso de delta destructivo**: los cuatro requerimientos siguientes, archivados el 2026-09-18 como parte de INC-19, se retiran en su totalidad junto con `internal/cli/odd_create.go`, `internal/cli/odd_status.go`, `internal/cli/odd_promote.go` y el despacho `case "odd":` de `cmd/axiom/main.go`. Tras esta retirada, el grupo de comandos `axiom odd` deja de existir; corresponde a la fase de archivado decidir si `openspec/specs/odd-cli-commands/spec.md` se elimina por completo o se conserva vacío con nota de retirada.

## REMOVED Requirements

### Requirement: Subcomando axiom odd create (REQ-19.5)

(Motivo: `internal/cli/odd_create.go` y el despacho `case "odd":` que lo invocaba desde `cmd/axiom/main.go` se retiran por la Decisión D1.)
(Migración: ninguna. La creación de un documento vivo ODD pasa a ser un acto de agente con herramientas de fichero, sin subcomando de CLI.)

### Requirement: Subcomando axiom odd status con Salida en Texto y JSON (REQ-19.6)

(Motivo: `internal/cli/odd_status.go` se retira por la Decisión D1.)
(Migración: ninguna. La consulta de progreso ODD pasa a leerse directamente del contenido Markdown del documento vivo, sin salida `--json` estructurada.)

### Requirement: Bandera --check-mirror en axiom odd status (REQ-19.7)

(Motivo: la bandera `--check-mirror` de `internal/cli/odd_status.go` se retira junto con el subcomando completo.)
(Migración: ninguna. No existe una comprobación de divergencia equivalente tras esta retirada.)

### Requirement: Subcomando axiom odd promote: Banderas y Presencia en la Ayuda (REQ-19.8)

(Motivo: `internal/cli/odd_promote.go` y la entrada del grupo `axiom odd` en `printHelp()` de `cmd/axiom/main.go` se retiran por la Decisión D1.)
(Migración: ver el delta de la capacidad `odd-sdd-promotion` en este mismo documento para el reemplazo funcional de la promoción.)

---

## 10. Capacidad Modificada (Retirada Destructiva): `odd-sdd-promotion`

**Aviso de delta destructivo**: los cuatro requerimientos siguientes, archivados el 2026-09-18 como parte de INC-19, se retiran en su totalidad junto con `internal/odd/promote.go`. Corresponde a la fase de archivado decidir si `openspec/specs/odd-sdd-promotion/spec.md` se elimina por completo o se conserva vacío con nota de retirada.

## REMOVED Requirements

### Requirement: Mapeo Determinista de Documento ODD a Propuesta SDD (REQ-19.9)

(Motivo: el mapeo determinista de secciones del documento vivo hacia `proposal.md` lo implementaba `internal/odd/promote.go`, retirado por la Decisión D1.)
(Migración: la promoción de un documento vivo ODD a una propuesta SDD pasa a ser un acto de agente: el agente redacta `proposal.md` informado por el documento vivo, sin un mapeo Go determinista. El endpoint `POST /api/increments` con `proposal_body` (capacidad `dashboard-sdd-orchestration`, REQ-15.1) sigue disponible como vía de siembra de la propuesta ya redactada, sin cambios por este incremento.)

### Requirement: Validación de Nombre y Colisión al Promover (REQ-19.10)

(Motivo: la validación de nombre y las guardas de colisión específicas de la promoción ODD→SDD las implementaba `internal/odd/promote.go`, retirado por la Decisión D1.)
(Migración: la validación de nombre y colisión ordinaria de `axiom change create` y del endpoint `POST /api/increments` sigue aplicándose sobre la propuesta resultante, sin cambios por este incremento.)

### Requirement: Prohibición de Fabricar Contenido de Especificación al Promover (REQ-19.11)

(Motivo: el marcador explícito de la sección `## Capacidades` para `sdd-spec` lo generaba `internal/odd/promote.go`, retirado por la Decisión D1.)
(Migración: ninguna. Queda a criterio del agente que redacta la propuesta no fabricar contenido de la sección `## Capacidades`.)

### Requirement: Estado del Documento ODD tras la Promoción (REQ-19.12)

(Motivo: el marcado del documento vivo como `promovido` y el rechazo de una segunda promoción los implementaba `internal/odd/promote.go`, retirado por la Decisión D1.)
(Migración: ninguna. El seguimiento de qué documento vivo ya se promovió pasa a ser una convención de agente sin verificación Go.)

---

## 11. Capacidad Modificada (Retirada Destructiva): `odd-ui-integration`

**Aviso de delta destructivo**: los tres requerimientos siguientes, archivados el 2026-09-18 como parte de INC-19, se retiran en su totalidad junto con `internal/dashboard/odd_service.go` y `internal/tui/screens/odd_features.go`, así como la entrada de ODD en `internal/tui/screens/governance.go` y su enrutamiento en `internal/tui/router.go`. Corresponde a la fase de archivado decidir si `openspec/specs/odd-ui-integration/spec.md` se elimina por completo o se conserva vacío con nota de retirada.

## REMOVED Requirements

### Requirement: Exposición del Estado ODD en el Dashboard Web (REQ-19.13)

(Motivo: `internal/dashboard/odd_service.go`, que exponía el estado ODD vía API REST local, se retira por la Decisión D1.)
(Migración: ninguna. El Dashboard Web deja de mostrar el estado de los documentos vivos ODD.)

### Requirement: Exposición del Estado ODD en la TUI (REQ-19.14)

(Motivo: `internal/tui/screens/odd_features.go` se retira por la Decisión D1.)
(Migración: ninguna. El progreso ODD deja de ser visible desde la TUI; solo es legible leyendo directamente el contenido Markdown del documento vivo.)

### Requirement: Conmutación Visible entre Carril ODD y Carril SDD (REQ-19.15)

(Motivo: la acción de conmutación de carril y el salto directo desde un documento promovido hacia su cambio SDD dependían de `internal/dashboard/odd_service.go` y de la entrada de ODD en `internal/tui/screens/governance.go`, retirados por la Decisión D1.)
(Migración: ninguna. No existe una conmutación de carril visible equivalente en el Dashboard Web ni en la TUI tras esta retirada; el carril SDD sigue siendo accesible por sus propias pantallas y endpoints, sin cambios por este incremento.)

# Especificación Viva: Identidad de Distribución de Axiom

> **Dominio:** `axiom-distribution-identity`  
> **Versión Canónica:** 1.0.0  
> **Estado:** Vigente  
> **Idioma:** Español (Castellano peninsular)

---

## 1. Capacidad: `axiom-distribution-identity`

Contrato de nombre publicado por clase de superficie: instalador y tap, compuertas de release, workflows de CI, shim de `crosslane`, namespace de protocolo en `contracts/**`, ruta de módulo Go y nombres de servicio de telemetría de despliegue. Distingue interoperabilidad (lo que lee una máquina, no renombrable sin romper consumidores) de identidad (lo que lee un humano, renombrable).

### Requirement: Taxonomía de interoperabilidad vs. identidad (REQ-20.7)

El sistema DEBE clasificar cada superficie de nombre publicado en exactamente una de dos categorías: interoperabilidad (contrato de cable consumido por otro software, cuyo renombrado rompe a consumidores) o identidad (superficie leída por una persona, renombrable sin romper ningún contrato). El namespace de protocolo bajo `contracts/**` y los marcadores de contenido gestionado que un componente Go analiza de forma literal DEBEN clasificarse como interoperabilidad. El instalador, el tap de Homebrew, las compuertas de release, el nombre de binario citado en la documentación y los nombres de servicio de despliegue DEBEN clasificarse como identidad.

#### Scenario: El namespace de contracts/** se clasifica como interoperabilidad
- **DADO** el fichero `contracts/review-integration/v2/schemas/status.schema.json` con `"$id": "https://gentle-ai.dev/contracts/review-integration/v2/schemas/status.schema.json"`
- **CUANDO** se clasifica esa superficie
- **ENTONCES** se clasifica como interoperabilidad
- **Y** ninguna tanda de identidad la modifica

---

### Requirement: Publicación y compuertas de release bajo identidad propia (REQ-20.8)

Las compuertas de release (`scripts/verify-release-assets.sh`, `scripts/release-preflight.sh`, `scripts/promote-stable-preflight.sh`) y la configuración de GoReleaser DEBEN apuntar a la identidad de distribución de Axiom (`IGutierrezZ/axiom`). Los artefactos de release publicados llevan el prefijo `axiom_{{ .Version }}_*`.

#### Scenario: Los activos publicados usan la identidad de Axiom
- **DADO** una versión taggeada del proyecto
- **CUANDO** el pipeline de release construye y publica los activos
- **ENTONCES** los binarios y paquetes se publican bajo el repositorio de Axiom con el nombre `axiom_*`
- **Y** el manifiesto de procedencia y los checksums firmados atestiguan el repositorio `IGutierrezZ/axiom`

---

### Requirement: Ruta de módulo Go migrada a /v3 (REQ-20.9)

La ruta de módulo Go DEBE migrar de `github.com/gentleman-programming/gentle-ai/v2` al destino que sigue a upstream, `github.com/gentleman-programming/gentle-ai/v3`. La migración DEBE ser una única reescritura mecánica aplicada a todos los ficheros que citan la ruta anterior.

#### Scenario: Migración mecánica completa de la ruta de módulo
- **DADO** el módulo declarado como `github.com/gentleman-programming/gentle-ai/v2` en `go.mod`
- **CUANDO** se ejecuta la migración
- **ENTONCES** `go.mod` y todo fichero que importaba la ruta `/v2` pasan a citar `/v3`
- **Y** `go build ./...` compila en verde tras la migración

---

### Requirement: Namespace de protocolo en contracts/** sin cambios (REQ-20.10)

El sistema NO DEBE modificar el namespace de protocolo bajo `contracts/**`: los valores `$id`, los prefijos de contrato (`gentle-ai.review-integration/v2`, `gentle-ai.sdd-status/v2` y equivalentes) y los esquemas asociados DEBEN permanecer byte a byte idénticos.

#### Scenario: contracts/** permanece sin cambios
- **DADO** el estado de `contracts/**/*.schema.json`
- **CUANDO** se auditan los esquemas
- **ENTONCES** cada fichero bajo `contracts/**` es byte a byte idéntico
- **Y** los agentes y revisores continúan interoperando con el formato publicado

---

### Requirement: Pasarela gentle-ai y shim de crosslane conservados (REQ-20.11)

El sistema DEBE conservar `cmd/gentle-ai` como pasarela de deprecación hacia `axiom`, y DEBE conservar sin cambios de comportamiento el shim ejecutable `gentle-ai` en `$PATH` que generan `scripts/crosslane/battery.go` y `scripts/crosslane/host.go`.

#### Scenario: cmd/gentle-ai emite su aviso de deprecación
- **DADO** el binario compilado desde `cmd/gentle-ai`
- **CUANDO** se invoca con cualquier argumento
- **ENTONCES** emite el aviso de deprecación hacia `axiom` en la salida de error estándar
- **Y** delega la ejecución al mismo comportamiento que `internal/app.RunArgs`

---

### Requirement: Nombres de servicio de telemetría de despliegue (REQ-20.12)

Los nombres de servicio de despliegue de telemetría (`deploy/telemetry/axiom-telemetry.service`, `deploy/telemetry/axiom-telemetry-backup.service` y paneles de Grafana asociados) DEBEN resolverse hacia la identidad de distribución de Axiom.

#### Scenario: Los nombres de servicio de telemetría se renombran
- **DADO** las definiciones bajo `deploy/telemetry/`
- **CUANDO** se despliega el servicio
- **ENTONCES** los ficheros de unidad y paneles de Grafana citan la identidad de Axiom

---

### Requirement: Raíz de respaldos resuelta exclusivamente a través de internal/backup (REQ-20.13)

Todo escritor de producción que cree o localice el directorio raíz de respaldos DEBE resolverlo invocando la función de resolución canónica exportada de `internal/backup` (`backup.BackupRootFor(homeDir)`), y NO DEBE construir esa ruta mediante un literal `filepath.Join` propio. Los respaldos ya existentes bajo `~/.gentle-ai/backups` DEBEN seguir siendo legibles por `ListBackups` sin migración obligatoria.

#### Scenario: Un escritor de producción resuelve la raíz a través del paquete
- **DADO** cualquier componente de backup, sync, run, upgrade o uninstall
- **CUANDO** necesita el directorio raíz de respaldos
- **ENTONCES** lo obtiene invocando la función de resolución canónica de `internal/backup`

---

### Requirement: Guarda ejecutable contra rutas literales de raíz de respaldos (REQ-20.14)

El sistema DEBE mantener una prueba automatizada que analice los ficheros de producción de `internal/` y que falle si alguno construye la raíz de respaldos mediante literales directos en lugar de invocar la función de resolución canónica de `internal/backup`.

#### Scenario: La guarda pasa cuando todos los escritores usan la función canónica
- **DADO** que todos los escritores de producción vigilados resuelven la raíz a través de `internal/backup`
- **CUANDO** se ejecuta `go test ./cmd/axiom/...`
- **ENTONCES** la guarda `TestUserStateRootsResolveThroughOwningPackage` pasa sin señalar ningún literal

<!-- Especificación Viva generada a partir de '2026-09-18-inc-19-odd-workflow-and-promotion' -->

# Especificación de Requerimientos: Comandos CLI para ODD (INC-19)

## Propósito

Definir de forma rigurosa, ejecutable y verificable los requerimientos funcionales y los escenarios BDD para la exposición del carril ágil de Organic Driven Development (ODD) a través de la línea de comandos canónica de Axiom: `axiom odd create`, `axiom odd status` y `axiom odd promote`, replicando el patrón manual ya empleado por `axiom sdd` y `axiom change`, con salida en texto y JSON, gestión de espejos en Engram y presencia explícita en la ayuda de la CLI.

---

## 1. Capacidad: `odd-cli-commands`

Permite a desarrolladores y agentes crear, consultar y promover documentos vivos ODD desde la línea de comandos canónica `axiom`, replicando el patrón manual ya empleado por `axiom sdd` y `axiom change`.

### Requirement: Subcomando axiom odd create (REQ-19.5)

La CLI `axiom` DEBE proveer el subcomando `axiom odd create <nombre>`, que crea el documento vivo descrito en REQ-19.1 y REQ-19.2. Al completarse con éxito, DEBE informar en la salida estándar la ruta del fichero creado y DEBE finalizar con código de salida `0`. Ante un nombre inválido o en colisión, DEBE finalizar con código de salida distinto de `0` y un mensaje de error descriptivo en la salida de error estándar.

#### Scenario: Creación exitosa desde la CLI

- **DADO** el binario `axiom`
- **CUANDO** el usuario ejecuta `axiom odd create gestion-inventario`
- **ENTONCES** el comando crea `odd/tasks/gestion-inventario.md`
- **Y** informa la ruta creada en stdout con código de salida `0`

#### Scenario: Nombre inválido o en colisión rechazado desde la CLI

- **DADO** un documento vivo existente en `odd/tasks/gestion-inventario.md`
- **CUANDO** el usuario ejecuta `axiom odd create gestion-inventario` de nuevo
- **ENTONCES** el comando finaliza con código de salida distinto de `0`
- **Y** emite un mensaje de error descriptivo en stderr sin modificar el fichero existente

---

### Requirement: Subcomando axiom odd status con Salida en Texto y JSON (REQ-19.6)

La CLI `axiom` DEBE proveer el subcomando `axiom odd status`, que lee siempre y solo los documentos vivos existentes bajo `odd/tasks/` para informar su progreso, DEBE ofrecer salida legible en texto por defecto y salida estructurada mediante la bandera `--json`, y en ningún caso DEBE escribir en Engram.

#### Scenario: Consulta de estado en texto legible

- **DADO** uno o más documentos vivos en `odd/tasks/` con progreso parcial
- **CUANDO** el usuario ejecuta `axiom odd status`
- **ENTONCES** el comando imprime en stdout el progreso de cada documento, en formato legible, leído únicamente de los ficheros
- **Y** no se produce ninguna escritura en Engram

#### Scenario: Consulta de estado en formato estructurado

- **DADO** los mismos documentos vivos
- **CUANDO** el usuario ejecuta `axiom odd status --json`
- **ENTONCES** el comando imprime en stdout un objeto JSON con el progreso y los metadatos de cada documento

---

### Requirement: Bandera --check-mirror en axiom odd status (REQ-19.7)

El subcomando `axiom odd status` DEBE aceptar la bandera opcional `--check-mirror`, que activa la comparación explícita descrita en REQ-19.4 entre cada documento vivo y su espejo en Engram. Sin esa bandera, el comando NO DEBE invocar el espejo. Los tres estados informables son `sincronizado`, `divergente` y `no disponible`; el estado `no disponible` NO DEBE alterar el código de salida del comando.

#### Scenario: Estado por defecto no invoca el espejo

- **DADO** un documento vivo con espejo divergente en Engram
- **CUANDO** el usuario ejecuta `axiom odd status` sin `--check-mirror`
- **ENTONCES** el comando informa el progreso leído del fichero
- **Y** no se invoca ninguna comparación contra Engram

#### Scenario: --check-mirror informa divergencia explícita

- **DADO** el mismo documento vivo con espejo divergente
- **CUANDO** el usuario ejecuta `axiom odd status --check-mirror`
- **ENTONCES** el comando informa el estado `divergente` para ese documento

#### Scenario: Engram no disponible no altera el código de salida

- **DADO** que el binario o servicio de Engram no responde
- **CUANDO** el usuario ejecuta `axiom odd status --check-mirror`
- **ENTONCES** el comando informa el estado `no disponible`
- **Y** conserva el mismo código de salida que habría tenido sin `--check-mirror`

---

### Requirement: Subcomando axiom odd promote: Banderas y Presencia en la Ayuda (REQ-19.8)

La CLI `axiom` DEBE proveer el subcomando `axiom odd promote <feature>`, con las banderas opcionales `--dry-run` (previsualiza sin escribir) y `--name <nombre>` (sobrescribe el nombre de cambio derivado del *feature*). El grupo `axiom odd` completo (`create`, `status`, `promote`) DEBE figurar en la salida de `axiom --help` / `printHelp()`.

#### Scenario: --dry-run previsualiza sin escribir

- **DADO** un documento vivo `odd/tasks/gestion-inventario.md` listo para promover
- **CUANDO** el usuario ejecuta `axiom odd promote gestion-inventario --dry-run`
- **ENTONCES** el comando imprime en stdout el contenido que tendría el `proposal.md` sembrado
- **Y** no crea ningún directorio ni fichero bajo `openspec/changes/`

#### Scenario: --name sobrescribe el nombre de cambio derivado

- **DADO** el mismo documento vivo
- **CUANDO** el usuario ejecuta `axiom odd promote gestion-inventario --name modulo-inventario-v2`
- **ENTONCES** el cambio SDD se crea en `openspec/changes/modulo-inventario-v2/`

#### Scenario: El grupo odd aparece en la ayuda de la CLI

- **DADO** el binario `axiom`
- **CUANDO** el usuario ejecuta `axiom --help`
- **ENTONCES** la salida incluye una entrada para `axiom odd` junto a sus subcomandos `create`, `status` y `promote`

---

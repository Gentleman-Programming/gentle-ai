# Especificación de Requerimientos: Identidad de Axiom y Topología de Workspace (INC-01)

## Propósito

Definir de forma exhaustiva y verificable los requerimientos funcionales y escenarios de comportamiento para el punto de entrada CLI de Axiom (`cmd/axiom`) y el motor de validación de topología de espacio de trabajo (`internal/workspace`).

---

## 1. Capacidad: `axiom-cli-identity`

El binario ejecutable `axiom` proporciona la interfaz de línea de comandos unificada para interactuar con la plataforma Axiom, desacoplada de Gentle-AI y con identidad de versión propia.

### Requirement: Información de Versión e Identidad (REQ-1.1)

El ejecutable `axiom` DEBE soportar la bandera `--version`, `-v` y el subcomando `version`, imprimiendo el nombre de la plataforma, la versión semántica y la información del release.

#### Scenario: Consulta de versión mediante flag --version
- **DADO** que el usuario dispone del binario `axiom` compilado
- **CUANDO** ejecuta `axiom --version`
- **ENTONCES** la salida contiene la cadena `axiom version` seguida de la versión semántica (ej. `v0.1.0`)
- **Y** el proceso finaliza con código de salida `0`

#### Scenario: Consulta de versión mediante subcomando version
- **DADO** que el usuario invoca la CLI
- **CUANDO** ejecuta `axiom version`
- **ENTONCES** la salida muestra la versión y metadatos de compilación (commit, arquitectura y fecha)
- **Y** el código de salida es `0`

---

### Requirement: Subcomando axiom workspace validate (REQ-1.2)

El binario `axiom` DEBE proporcionar el subcomando `axiom workspace validate`, el cual evalúa el archivo `axiom.yaml` del directorio actual (o de la ruta especificada mediante `--path`) y verifica la conformidad de la topología y de los repositorios asociados a los roles.

#### Scenario: Validación de espacio de trabajo conforme
- **DADO** un directorio con un archivo `axiom.yaml` válido y las carpetas de repositorios existentes
- **CUANDO** el usuario ejecuta `axiom workspace validate`
- **ENTONCES** la salida reporta un estado `COMPLIANT` indicando la topología detectada y los roles verificados
- **Y** el código de salida del proceso es `0`

#### Scenario: Validación con ruta explícita mediante --path
- **DADO** un espacio de trabajo ubicado en una ruta externa `/tmp/test-workspace`
- **CUANDO** el usuario ejecuta `axiom workspace validate --path /tmp/test-workspace`
- **ENTONCES** la validación se ejecuta sobre el directorio especificado
- **Y** reporta el resultado correspondiente a esa ubicación

#### Scenario: Espacio de trabajo no conforme finaliza con error
- **DADO** un espacio de trabajo donde faltan repositorios obligatorios o el archivo de configuración es inválido
- **CUANDO** el usuario ejecuta `axiom workspace validate`
- **ENTONCES** la salida detalla de forma explícita en español los motivos de no conformidad
- **Y** el proceso finaliza con código de salida `1`

---

## 2. Capacidad: `workspace-topology-engine`

El paquete `internal/workspace` implementa la lógica de dominio para la lectura de `axiom.yaml` y la verificación determinista de la estructura de repositorios y roles bajo la carpeta maestra.

### Requirement: Estructura del Esquema axiom.yaml (REQ-2.1)

El motor de configuración DEBE deserializar y validar el archivo `axiom.yaml`, exigiendo los campos obligatorios:
- `workspace.name` (cadena no vacía)
- `workspace.topology` (uno de: `monorepo-embedded`, `monorepo-decoupled`, `multirepo`)
- `workspace.specs_repository` (ruta relativa al directorio de especificaciones)
- `roles` (mapa con al menos 1 rol definido, conteniendo `name` y una lista de `repositories`)

#### Scenario: Archivo axiom.yaml completo y bien formado
- **DADO** un contenido YAML con todos los campos obligatorios
- **CUANDO** se invoca `LoadConfig`
- **ENTONCES** se obtiene una estructura `WorkspaceConfig` poblada correctamente sin errores

#### Scenario: Topología desconocida o ausente
- **DADO** un archivo `axiom.yaml` con `workspace.topology: "desconocido"`
- **CUANDO** se intenta cargar o validar la configuración
- **ENTONCES** se devuelve un error descriptivo indicando que la topología debe ser `monorepo-embedded`, `monorepo-decoupled` o `multirepo`

#### Scenario: Lista de roles vacía
- **DADO** un archivo `axiom.yaml` sin ningún rol declarado
- **CUANDO** se valida la configuración
- **ENTONCES** se devuelve un error indicando que al menos un rol de desarrollo DEBE estar configurado

---

### Requirement: Topología Monorepo Embebido (REQ-2.2)

En la topología `monorepo-embedded`, el repositorio de especificaciones DEBE apuntar a la raíz del propio monorrepo (`"."`) o a una subcarpeta interna existente (como `openspec` o `specs`).

#### Scenario: Monorepo embebido con specs en raíz o subdirectorio válido
- **DADO** una configuración con topología `monorepo-embedded` y `specs_repository: "."`
- **Y** la carpeta existe dentro del espacio de trabajo
- **CUANDO** se ejecuta la validación de topología
- **ENTONCES** la topología se marca como válida y conforme

---

### Requirement: Topología Monorepo Desacoplado (REQ-2.3)

En la topología `monorepo-decoupled`, el repositorio de especificaciones DEBE residir en un directorio independiente dentro de la carpeta maestra común, distinto del repositorio de código del monorrepo.

#### Scenario: Monorepo desacoplado con repositorio de specs presente
- **DADO** una carpeta maestra que contiene `app-monorepo/` y `especificacion/`
- **Y** `specs_repository` apunta a `especificacion`
- **CUANDO** se ejecuta la validación
- **ENTONCES** la validación es exitosa

#### Scenario: Monorepo desacoplado sin repositorio de specs
- **DADO** una configuración `monorepo-decoupled` donde la ruta indicada en `specs_repository` no existe en el disco
- **CUANDO** se ejecuta la validación
- **ENTONCES** se genera un error de validación indicando que el repositorio de especificaciones es obligatorio y no fue encontrado

---

### Requirement: Topología Multirepo Federado (REQ-2.4)

En la topología `multirepo`, múltiples repositorios de código coexisten bajo la carpeta maestra común. Es REQUISITO OBLIGATORIO que exista un repositorio canónico de especificaciones dentro de dicha carpeta.

#### Scenario: Multirepo conforme con todos sus repositorios
- **DADO** una carpeta maestra con `especificacion/`, `frontend/` y `backend/`
- **Y** la configuración mapea los roles a `frontend` y `backend`, con `specs_repository: "especificacion"`
- **CUANDO** se ejecuta la validación
- **ENTONCES** el informe declara todos los repositorios y la topología como válidos

#### Scenario: Multirepo sin repositorio canónico de especificaciones
- **DADO** un proyecto multirrepositorio donde el directorio de `specs_repository` no existe
- **CUANDO** se ejecuta la validación de topología
- **ENTONCES** la validación es rechazada con un error crítico indicando que los proyectos multirrepositorio requieren obligatoriamente un repositorio canónico de especificación

#### Scenario: Repositorio asignado a un rol no existe físicamente
- **DADO** un rol `devops` configurado para gestionar la ruta `infra/k8s`
- **Y** la carpeta `infra/k8s` no existe dentro de la carpeta maestra
- **CUANDO** se ejecuta la validación
- **ENTONCES** la validación falla reportando específicamente qué ruta de qué rol no ha sido localizada en el sistema de archivos

---

### Requirement: Reporte de Validación Estructurado (REQ-2.5)

La función `ValidateTopology` DEBE retornar un objeto estructurado `ValidationReport` que contenga:
- `Valid` (booleano: `true` si cumple todas las reglas, `false` si hay errores)
- `Topology` (tipo de topología evaluada)
- `CheckedPaths` (lista de rutas verificadas)
- `Errors` (lista de mensajes de error explicativos en español)
- `Warnings` (lista de advertencias o recomendaciones, si aplican)

#### Scenario: Generación de reporte completo en caso de fallo
- **DADO** una ejecución de validación con errores
- **CUANDO** se obtiene el resultado
- **ENTONCES** el objeto `ValidationReport` contiene `Valid: false`, la lista de errores y las rutas inspeccionadas

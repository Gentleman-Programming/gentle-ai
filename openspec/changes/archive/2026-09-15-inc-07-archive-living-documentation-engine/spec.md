# Especificación de Requerimientos: Motor de Documentación Viva y Adopción Orgánica en Archive (INC-07)

## Propósito

Definir de forma ejecutable, determinista y verificable los requerimientos funcionales, no funcionales y escenarios BDD para transformar la fase de cierre (`Archive`) en un mantenedor continuo de especificaciones vivas y en un motor de adopción orgánica (*Zero-Doc Cold Start*) en Axiom: indexador canónico de especificaciones vivas (`openspec/specs/`), generador determinista del catálogo maestro `openspec/INDEX.md`, sintetizador orgánico para proyectos legados, subcomandos CLI `axiom archive` y la extensión visual del Dashboard Web local (`axiom ui`).

---

## 1. Capacidad: `livingdoc-spec-indexer`

El paquete `internal/livingdoc` provee capacidades de inspección, parseo y catalogación continua de todas las especificaciones vivas consolidadas en el workspace (`openspec/specs/`).

### Requirement: Escaneo y Extracción de Especificaciones Vivas (REQ-1.1)

El indexador DEBE recorrer recursivamente el directorio `openspec/specs/` (o la ruta configurada en `axiom.yaml` como `specs_repository`), localizando todos los archivos `spec.md`. Para cada archivo DEBE extraer:
- Identificador de dominio (nombre de la subcarpeta).
- Título principal de la especificación (encabezado `# `).
- Propósito o descripción introductoria.
- Lista completa de requerimientos canónicos (`### Requirement: ...`).
- Lista de escenarios BDD vinculados (`#### Scenario: ...`).

#### Scenario: Indexación exitosa de especificaciones vivas existentes
- **DADO** un directorio `openspec/specs/` con especificaciones vivas (`workspace-topology`, `structured-handoffs`, `multi-role-fan-out`, etc.)
- **CUANDO** el indexador procesa el repositorio de especificaciones
- **ENTONCES** genera una estructura `LivingCatalog` con una entrada `LivingSpecEntry` por cada dominio
- **Y** cada entrada reporta su recuento exacto de requerimientos y escenarios BDD

#### Scenario: Directorio de especificaciones vacío o inexistente
- **DADO** un proyecto que parte de cero sin la carpeta `openspec/specs/`
- **CUANDO** se invoca el indexador
- **ENTONCES** retorna un catálogo vacío con 0 especificaciones sin generar errores fatales

---

### Requirement: Generación Determinista de `openspec/INDEX.md` (REQ-1.2)

El motor DEBE ser capaz de compilar y escribir un documento canónico `openspec/INDEX.md` que resuma el estado consolidado de la arquitectura y contratos vivos del sistema. El índice DEBE incluir:
1. Encabezado maestro y fecha/hora de la última sincronización.
2. Tabla resumen de especificaciones vivas con: Dominio, Título, Requerimientos, Escenarios y Enlace relativo al archivo `spec.md`.
3. Desglose por dominios con el listado de requerimientos canónicos y sus escenarios.

#### Scenario: Generación y persistencia de `openspec/INDEX.md`
- **DADO** un catálogo de especificaciones vivas indexado
- **CUANDO** el servicio ejecuta el método de sincronización de índice
- **ENTONCES** escribe el archivo `openspec/INDEX.md` en el sistema de archivos
- **Y** todos los enlaces a las especificaciones son relativos y navegables

---

### Requirement: Detección de Inconsistencias o Requerimientos Huérfanos (REQ-1.3)

Al indexar, el motor DEBE advertir si alguna especificación carece de título, contiene requerimientos sin escenarios BDD asociados o tiene identificadores de requerimiento duplicados.

#### Scenario: Advertencia ante requerimiento sin escenarios
- **DADO** un archivo `spec.md` con un encabezado `### Requirement: ...` que no contiene ningún `#### Scenario:`
- **CUANDO** se ejecuta la indexación
- **ENTONCES** el catálogo registra el requerimiento pero añade una advertencia en `SyncReport`

---

## 2. Capacidad: `livingdoc-organic-coldstart`

El motor de adopción orgánica permite introducir SDD en proyectos existentes sin documentación previa (*Zero-Doc Cold Start*).

### Requirement: Síntesis de Especificación Viva a partir de un Cambio Archivado (REQ-2.1)

Cuando un incremento concluye en un proyecto donde el dominio correspondiente aún no cuenta con especificación viva en `openspec/specs/<dominio>/`, el sintetizador DEBE tomar los artefactos del incremento (`spec.md`, `proposal.md`, `verify-report.md`) y generar de forma determinista la especificación viva inicial en `openspec/specs/<dominio>/spec.md`.

#### Scenario: Adopción orgánica de un nuevo dominio
- **DADO** un cambio archivado o en fase de cierre para el dominio `billing-service`
- **Y** que no existe previamente `openspec/specs/billing-service/spec.md`
- **CUANDO** se invoca la operación de *Cold Start*
- **ENTONCES** se crea la carpeta `openspec/specs/billing-service/`
- **Y** se genera el archivo `spec.md` inicial con los requerimientos y contratos validados en el cambio

---

### Requirement: Enriquecimiento Semántico del Catálogo Vivo (REQ-2.2)

El sintetizador DEBE apoyarse en el motor semántico de Axiom (`internal/semantic`) para asociar a la especificación viva los símbolos clave (interfaces y structs principales) implementados en el código correspondiente a dicho dominio.

#### Scenario: Vinculación de contratos de interfaces al dominio vivo
- **DADO** un dominio de especificación y código Go analizado semánticamente
- **CUANDO** se sintetiza o actualiza la especificación viva
- **ENTONCES** se documentan los tipos y contratos exportados en la sección técnica de la spec viva

---

## 3. Capacidad: `axiom-cli-archive`

La CLI de Axiom en `cmd/axiom/main.go` expone el grupo de comandos `axiom archive`.

### Requirement: Comando `axiom archive sync` (REQ-3.1)

El comando `axiom archive sync [--path <dir>]` DEBE:
1. Re-escanear todas las especificaciones vivas en `openspec/specs/`.
2. Generar y sobrescribir de forma determinista el archivo `openspec/INDEX.md`.
3. Imprimir por consola el resumen con el total de especificaciones, requerimientos y escenarios sincronizados.

#### Scenario: Sincronización exitosa desde CLI
- **DADO** un workspace de Axiom con especificaciones vivas
- **CUANDO** el usuario ejecuta `axiom archive sync`
- **ENTONCES** se actualiza `openspec/INDEX.md`
- **Y** la salida por consola muestra el informe de sincronización con código de salida 0

---

### Requirement: Comando `axiom archive list` (REQ-3.2)

El comando `axiom archive list [--path <dir>]` DEBE presentar una tabla formateada con las especificaciones vivas activas, sus títulos, cantidad de requerimientos y escenarios BDD.

#### Scenario: Listado de especificaciones en consola
- **DADO** un workspace con especificaciones vivas
- **CUANDO** el usuario ejecuta `axiom archive list`
- **ENTONCES** se imprime una tabla con las columnas DOMINIO, TÍTULO, REQS, ESCENARIOS y RUTA

---

### Requirement: Comando `axiom archive show` (REQ-3.3)

El comando `axiom archive show <dominio> [--path <dir>]` DEBE mostrar en consola el detalle completo de la especificación viva para el dominio solicitado, incluyendo su propósito, lista de requerimientos y escenarios.

#### Scenario: Consulta de una especificación viva por dominio
- **DADO** una especificación viva existente para `workspace-topology`
- **CUANDO** el usuario ejecuta `axiom archive show workspace-topology`
- **ENTONCES** la CLI imprime el detalle estructurado de la especificación y sus requerimientos

---

### Requirement: Comando `axiom archive coldstart` (REQ-3.4)

El comando `axiom archive coldstart <cambio> [--domain <dominio>] [--path <dir>]` DEBE promover un cambio verificado a especificación viva cuando no existía previamente.

#### Scenario: Ejecución de coldstart para un cambio nuevo
- **DADO** un cambio completado con sus artefactos `spec.md` y `verify-report.md`
- **CUANDO** el usuario ejecuta `axiom archive coldstart inc-01-workspace`
- **ENTONCES** se consolida la especificación viva en `openspec/specs/`
- **Y** se ejecuta automáticamente la sincronización de `openspec/INDEX.md`

---

## 4. Capacidad: `axiom-dashboard-living-specs`

El Dashboard Web local (`internal/dashboard/`) expone la documentación viva a través de su API REST JSON y la renderiza en una nueva vista de la interfaz SPA.

### Requirement: Endpoints REST para Especificaciones Vivas (REQ-4.1)

El servidor web DEBE exponer las siguientes rutas REST:
- `GET /api/archive/specs`: Retorna el catálogo completo `LivingCatalog` serializado en JSON.
- `GET /api/archive/specs/{domain}`: Retorna el contenido estructurado y texto Markdown de la spec viva del dominio solicitado.
- `POST /api/archive/sync`: Ejecuta la re-indexación y sincronización de `openspec/INDEX.md` desde la Web UI.

#### Scenario: Petición HTTP a `/api/archive/specs`
- **DADO** el servidor del Dashboard Web en ejecución
- **CUANDO** se efectúa una petición `GET` a `/api/archive/specs`
- **ENTONCES** responde con código HTTP 200 y cabecera `Content-Type: application/json`
- **Y** el cuerpo contiene la lista de especificaciones vivas con sus requerimientos

---

### Requirement: Interfaz Web SPA con Explorador de Documentación Viva (REQ-4.2)

La interfaz SPA embebida (`assets/index.html`, `app.js`, `style.css`) DEBE incluir una nueva pestaña de navegación **"Especificaciones Vivas"** que permita:
1. Visualizar tarjetas de resumen con el total de especificaciones vivas, requerimientos y escenarios BDD consolidados en el sistema.
2. Disponer de un botón de acción rápida para re-sincronizar el catálogo (`↻ Sincronizar Catálogo`).
3. Navegar interactivamente por los dominios de especificación y leer su contenido formateado con visor de requerimientos y escenarios.

#### Scenario: Exploración de especificaciones vivas en el navegador
- **DADO** el usuario con el Dashboard abierto en la pestaña "Especificaciones Vivas"
- **CUANDO** selecciona una especificación de la lista
- **ENTONCES** se carga el detalle del dominio con su propósito, requerimientos y escenarios BDD
- **Y** se provee un botón para sincronizar el índice en un solo clic

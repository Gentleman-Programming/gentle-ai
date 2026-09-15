# Especificación de Requerimientos: Autoskills (midudev/autoskills) y Minería Heurística con Gobernanza Human-in-the-Loop

## Propósito

Definir de forma rigurosa, ejecutable y verificable los requerimientos funcionales, no funcionales y escenarios BDD para la integración del catálogo oficial de `midudev/autoskills`, el cliente nativo en Go con verificación criptográfica SHA-256, el motor de minería heurística de repositorio, el buzón transitorio de gobernanza *Human-in-the-Loop* (`.axiom/skills/inbox/`), los subcomandos de la CLI `axiom skill` y la extensión del Dashboard Web local en el runtime de Axiom.

---

## 1. Capacidad: `autoskill-midudev-registry`

El paquete `internal/autoskill` provee un cliente nativo en Go para interactuar con el registro oficial auditado de `midudev/autoskills` alojado en GitHub (`https://raw.githubusercontent.com/midudev/autoskills/main/packages/autoskills/skills-registry/`).

### Requirement: Cliente HTTP Nativo y Parseo de Índice de Registro (REQ-1.1)

El cliente DEBE descargar y deserializar el archivo de manifiesto `index.json` del registro oficial de `midudev/autoskills`. El manifiesto contiene la lista de skills auditadas, sus rutas de origen, la lista de archivos asociados, sus hashes SHA-256 esperados y la metadata del informe de revisión de seguridad.

#### Scenario: Descarga y parseo exitoso del índice de registro
- **DADO** conectividad de red con el repositorio oficial de `midudev/autoskills`
- **CUANDO** el cliente invoca el método de consulta del índice del registro
- **ENTONCES** se obtiene una estructura `RegistryIndex` con la versión del manifiesto, la fecha de generación y el mapa de skills registradas
- **Y** cada entrada de skill contiene la lista de archivos y su respectiva tabla de hashes SHA-256

#### Scenario: Manejo de fallo de red en la consulta del registro
- **DADO** un entorno sin conexión a internet o un endpoint no disponible
- **CUANDO** el cliente intenta consultar el índice remoto
- **ENTONCES** el cliente recurre a la caché local en `.axiom/cache/autoskills/index.json` si existe, o retorna un error descriptivo indicando indisponibilidad de red

---

### Requirement: Verificación Criptográfica Estricta de Integridad SHA-256 (REQ-1.2)

Al descargar cualquier archivo perteneciente a una skill del registro (incluyendo `SKILL.md` y scripts o recursos complementarios), el cliente DEBE computar su suma de verificación **SHA-256** mediante `crypto/sha256` y compararla con el hash declarado en el manifiesto `index.json`. Si los hashes no coinciden, la descarga DEBE ser rechazada de forma inmediata y no debe depositarse en el buzón.

#### Scenario: Descarga con hash SHA-256 coincidente
- **DADO** un archivo de skill descargado desde el registro oficial
- **CUANDO** el cliente calcula el hash SHA-256 de los bytes recibidos y lo compara con el declarado en `index.json`
- **ENTONCES** los hashes coinciden exactamente
- **Y** el archivo se considera auténtico y apto para ser propuesto al buzón

#### Scenario: Detección y rechazo de archivo corrompido o alterado
- **DADO** una respuesta HTTP cuyo contenido ha sido alterado o truncado en tránsito
- **CUANDO** el cliente calcula el hash SHA-256 de los bytes recibidos
- **ENTONCES** el cliente detecta una discrepancia con el hash esperado de `index.json`
- **Y** aborta la operación retornando un error de integridad criptográfica sin escribir el fichero en disco

---

### Requirement: Modo Offline y Caché Local (REQ-1.3)

El sistema DEBE soportar la bandera `--offline` tanto en la CLI como en las llamadas al servicio, utilizando los archivos previamente descargados en `.axiom/cache/autoskills/` o definiciones preempaquetadas para operar sin realizar peticiones HTTP salientes.

#### Scenario: Escaneo en modo offline utilizando caché local
- **DADO** un directorio `.axiom/cache/autoskills/` con un índice y skills previamente sincronizados
- **CUANDO** se invoca el escaneo con la opción `--offline`
- **ENTONCES** no se ejecuta ninguna petición HTTP de red
- **Y** la resolución de skills se efectúa íntegramente desde la caché local

---

## 2. Capacidad: `autoskill-stack-detector`

El motor de detección analiza el código fuente y las configuraciones de los repositorios configurados en `axiom.yaml` contra la matriz de tecnologías `SKILLS_MAP`.

### Requirement: Detección de Stack Multi-Rol basada en `axiom.yaml` (REQ-2.1)

El detector DEBE cargar la configuración del workspace desde `axiom.yaml`, iterar sobre todos los roles configurados y evaluar las tecnologías declaradas o presentes en las rutas de sus repositorios asociados.

#### Scenario: Detección de tecnologías en repositorios de roles
- **DADO** un archivo `axiom.yaml` con roles asociados a directorios de repositorios locales
- **CUANDO** se ejecuta el detector de tecnologías
- **ENTONCES** se identifican las tecnologías de cada rol evaluando dependencias en `package.json`, `go.mod`, `Cargo.toml` u otros manifiestos
- **Y** cada tecnología detectada se vincula con el rol correspondiente y con sus skills asociadas según `SKILLS_MAP`

---

### Requirement: Reglas de Detección basadas en `SKILLS_MAP` (REQ-2.2)

El detector DEBE evaluar las reglas de `SKILLS_MAP` considerando:
1. Nombres de paquetes en listas de dependencias (`packages`).
2. Archivos de configuración característicos en el árbol de directorios (`configFiles`).
3. Extensiones de archivo presentes en el repositorio (`fileExtensions`).

#### Scenario: Detección de framework web por archivo de configuración y dependencias
- **DADO** un repositorio que contiene `next.config.mjs` y `package.json` con dependencia `next`
- **CUANDO** el detector procesa el directorio
- **ENTONCES** se detecta la tecnología `nextjs`
- **Y** se proponen las skills asociadas (`next-best-practices`, etc.) para el rol correspondiente

---

## 3. Capacidad: `autoskill-heuristic-miner`

El analizador heurístico examina el código fuente local de los repositorios vinculados para extraer patrones arquitectónicos y convenciones internas propias del proyecto Axiom.

### Requirement: Minería de Convenciones en Código Go (REQ-3.1)

El minero DEBE inspeccionar archivos `.go` del repositorio e identificar patrones recurrentes, incluyendo:
1. Estructura de pruebas tabulares (*table-driven tests* con `tests := []struct{...}`).
2. Organización de paquetes internos en `internal/` (Clean Architecture / encapsulación).
3. Manejo y propagación idiomática de errores en Go mediante formato `fmt.Errorf("...: %w", err)`.

#### Scenario: Detección de pruebas tabulares en Go
- **DADO** un repositorio en Go que contiene suites de pruebas estructuradas como pruebas tabulares
- **CUANDO** el minero heurístico analiza los archivos `*_test.go`
- **ENTONCES** reconoce el patrón `table-driven-tests`
- **Y** formula una propuesta de skill local `axiom-go-table-tests`

---

### Requirement: Generación Canónica de Borradores de Skill Minada (REQ-3.2)

Cada patrón detectado por el minero DEBE formularse como un documento canónico `SKILL.md` que incluya:
1. Encabezado Frontmatter con `name`, `description`, `trigger` y `origin: mined`.
2. Sección de Propósito y Reglas del patrón en español.
3. Ejemplos de código correctos (*Do*) e incorrectos (*Don't*) basados en la estructura del propio repositorio.

#### Scenario: Generación estructurada de borrador de skill minada
- **DADO** un patrón de arquitectura detectado por el minero
- **CUANDO** se genera la propuesta de skill
- **ENTONCES** el archivo `SKILL.md` resultante sigue estrictamente el formato canónico de skills de Axiom
- **Y** queda redactado en español castellano con ejemplos del repositorio

---

## 4. Capacidad: `autoskill-governance-inbox`

El buzón transitorio implementa la compuerta de gobernanza *Human-in-the-Loop*. Ninguna skill se instala directamente en producción sin revisión y aprobación humana explícita.

### Requirement: Depósito de Propuestas en Bandeja de Entrada Transitoria (REQ-4.1)

Tanto las skills detectadas desde el registro de `midudev` como las minadas localmente DEBEN depositarse en la ruta transitoria `.axiom/skills/inbox/<nombre>/`, compuesta por:
1. `SKILL.md`: El contenido completo de la directriz.
2. `metadata.json`: Metadatos que certifican el origen (`midudev` o `mined`), URL o fuente, suma SHA-256, estado de verificación criptográfica (`verified: true`), rol asignado, fecha de propuesta y justificación de detección.

#### Scenario: Depósito de propuesta de skill en el buzón
- **DADO** un escaneo que descubre una nueva skill aplicable
- **CUANDO** el gestor de buzón procesa la coincidencia
- **ENTONCES** se crea la carpeta `.axiom/skills/inbox/<nombre>/`
- **Y** se escriben los archivos `SKILL.md` y `metadata.json` con su estado pendiente de aprobación

---

### Requirement: Aprobación y Promoción Atómica a Producción (REQ-4.2)

Al invocar la aprobación de una skill (`axiom skill approve <nombre>` o vía API REST), el sistema DEBE:
1. Validar la existencia de la propuesta en `.axiom/skills/inbox/<nombre>/`.
2. Mover o copiar de forma atómica el archivo `SKILL.md` (y recursos si existieran) al directorio canónico activo `skills/<nombre>/SKILL.md`.
3. Eliminar la propuesta de la bandeja transitoria `.axiom/skills/inbox/<nombre>/`.

#### Scenario: Aprobación exitosa de una skill propuesta
- **DADO** una propuesta existente en `.axiom/skills/inbox/react-best-practices/`
- **CUANDO** un usuario ejecuta `axiom skill approve react-best-practices`
- **ENTONCES** el archivo se promociona a `skills/react-best-practices/SKILL.md`
- **Y** la propuesta se remueve de `.axiom/skills/inbox/`
- **Y** la skill queda inmediatamente activa para el proyecto y sus agentes

#### Scenario: Rechazo de aprobación si la skill no existe en el buzón
- **DADO** una petición de aprobación para una skill inexistente en el buzón
- **CUANDO** se invoca la aprobación
- **ENTONCES** el sistema devuelve un error claro indicando que no existe tal propuesta en el buzón

---

### Requirement: Rechazo y Purga de Propuestas (REQ-4.3)

Al invocar el rechazo de una skill (`axiom skill reject <nombre>` o vía API REST), el sistema DEBE eliminar por completo el directorio `.axiom/skills/inbox/<nombre>/` sin alterar en modo alguno el directorio canónico `skills/`.

#### Scenario: Descarte de una propuesta del buzón
- **DADO** una propuesta en `.axiom/skills/inbox/skill-descartable/`
- **CUANDO** se invoca `axiom skill reject skill-descartable`
- **ENTONCES** el directorio de la propuesta es purgado de `.axiom/skills/inbox/`
- **Y** el directorio de skills activas `skills/` permanece inalterado

---

## 5. Capacidad: `axiom-cli-skill`

El binario de línea de comandos `axiom` incorpora el grupo de comandos `axiom skill` para gestionar el ciclo de vida de las skills desde la terminal.

### Requirement: Subcomandos de la CLI `axiom skill` (REQ-5.1)

El comando `axiom skill` DEBE soportar los siguientes subcomandos:
1. `axiom skill scan [--role <rol>] [--path <directorio>] [--offline]`: Ejecuta la detección contra `midudev/autoskills` y la minería heurística, poblando el buzón transitorio.
2. `axiom skill list [--inbox] [--path <directorio>]`: Lista las skills activas en el proyecto o, con la bandera `--inbox`, lista las propuestas pendientes de revisión con su procedencia y estado SHA-256.
3. `axiom skill approve <nombre> [--path <directorio>]`: Aprueba e instala una skill en `skills/`.
4. `axiom skill reject <nombre> [--path <directorio>]`: Rechaza y elimina una propuesta del buzón.

#### Scenario: Ejecución de escaneo por línea de comandos
- **DADO** un workspace con configuración de roles en `axiom.yaml`
- **CUANDO** se ejecuta `axiom skill scan`
- **ENTONCES** la CLI muestra en consola las tecnologías detectadas, las skills consultadas y el número de propuestas depositadas en el buzón

#### Scenario: Listado del buzón por línea de comandos
- **DADO** un buzón con propuestas pendientes
- **CUANDO** se ejecuta `axiom skill list --inbox`
- **ENTONCES** la CLI imprime una tabla formateada con: Nombre de la Skill, Origen (`midudev` / `mined`), Estado SHA-256 (`VERIFICADO`) y Rol asociado

---

## 6. Capacidad: `axiom-dashboard-skill-inbox`

El servidor local y el frontend web de Axiom extienden sus funcionalidades para ofrecer gestión interactiva del buzón de skills en un solo clic.

### Requirement: Endpoints REST del Buzón de Skills (REQ-6.1)

El servidor en `internal/dashboard/server.go` DEBE exponer las siguientes rutas REST:
1. `GET /api/skills/inbox`: Retorna la lista JSON de propuestas en el buzón con su metadata y contenido.
2. `POST /api/skills/scan`: Ejecuta el escaneo de tecnologías y minería, retornando el reporte de detección.
3. `POST /api/skills/approve`: Recibe `{"name": "nombre"}` y aprueba la skill especificada.
4. `POST /api/skills/reject`: Recibe `{"name": "nombre"}` y descarta la propuesta especificada.

#### Scenario: Consulta de propuestas vía API REST
- **DADO** una o más propuestas en `.axiom/skills/inbox/`
- **CUANDO** un cliente realiza una petición `GET /api/skills/inbox`
- **ENTONCES** el servidor responde con código `200 OK` y un array JSON de propuestas con sus atributos completos

---

### Requirement: Interfaz Web SPA para el Buzón de Skills (REQ-6.2)

La pestaña de "Skills" en el dashboard web DEBE incluir un panel interactivo del "Buzón de Autoskills (Gobernanza Human-in-the-Loop)", permitiendo a los desarrolladores visualizar la procedencia de cada skill (insignia `midudev auditado` o `minería local`), su hash SHA-256 verificado, inspeccionar el contenido markdown y pulsar los botones "Aprobar" o "Rechazar" con actualización reactiva inmediata de la vista.

#### Scenario: Aprobación visual de una skill desde la interfaz web
- **DADO** una propuesta mostrada en el panel del buzón en la interfaz web
- **CUANDO** el usuario presiona el botón "Aprobar" en la tarjeta de la skill
- **ENTONCES** la interfaz envía una petición `POST /api/skills/approve`
- **Y** al recibir respuesta exitosa, actualiza la lista de skills activas y remueve la tarjeta del buzón

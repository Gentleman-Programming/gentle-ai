# Especificación: Desacoplamiento de RDD y Parches de Estabilidad Upstream v3 (inc-18-rdd-decoupling-and-v3-stability-fixes)

> **Incremento:** `inc-18-rdd-decoupling-and-v3-stability-fixes`  
> **Fase:** Fase 4 — Evolución Arquitectónica, Flujo Orgánico (ODD) y Absorción Upstream v3  
> **Responsabilidad:** Core SDD / Estabilidad & Plataforma  
> **Estado:** En desarrollo  
> **Idioma:** Español (Castellano peninsular)  

---

## 1. Contexto y Justificación

Axiom nació como un fork y evolución paralela de Gentle-AI. En upstream, el motor de estados de Spec-Driven Development (SDD) heredó un acoplamiento estrecho con Review-Driven Development (RDD), llegando a inyectar ofertas forzadas de revisión (`reviewOffer`) e invocaciones sugeridas de CLI tras cada verificación exitosa, condicionando la percepción del ciclo de vida y añadiendo sobrecarga innecesaria.

Asimismo, la versión de upstream Gentle-AI v3 (v3.0.2+) introdujo una serie de correcciones de estabilidad y arquitectura críticas que Axiom debe absorber:
1. Fallos en tests y aserciones de rutas bajo Windows debido a backslashes sin decodificar en salidas JSON de SDD (`1a2f6775`).
2. Rutas de artefactos de configuración de agentes que dependían erróneamente del directorio de trabajo actual (`CWD`) del proceso (`8c078527`).
3. Fuga de skills internas de colaboradores del repositorio en los presets predeterminados de usuarios generales (`11f6c000`).
4. Fragilidad en el apretón de manos inicial del protocolo Engram MCP ante el error `ambiguous_project` en entornos multi-proyecto (`59e6705f`, `90992285`).

---

## 2. Requerimientos Canónicos y Escenarios BDD

### Requirement: Desacoplamiento de RDD en la Proyección de Estado SDD (REQ-18.1)
El motor de estados `internal/sddstatus` debe eliminar de forma limpia la inyección forzada del bloque `reviewOffer` y llamadas acopladas a transacciones de revisión. La preparación para el archivado (`archive: ready`) debe depender estrictamente de la compleción de tareas y el veredicto favorable en `verify-report.md`.

#### Scenario: Proyección de estado SDD tras verificación exitosa sin reviewOffer forzado
- **GIVEN** un incremento en curso en fase de verificación con tareas completadas al 100% y `verify-report.md` con veredicto PASS.
- **WHEN** se ejecuta la proyección de estado `axiom sdd status [cambio] --json`.
- **THEN** el documento JSON emitido debe omitir la clave acoplada `reviewOffer`.
- **AND** el campo `dependencies.archive` debe evaluarse como `ready` sin exigir interacciones o autorizaciones previas de RDD.

---

### Requirement: Independencia Operativa de las Herramientas RDD (REQ-18.2)
Las herramientas y subcomandos de revisión RDD (`axiom review ...`) deben permanecer completamente funcionales como comandos opt-in independientes gobernados por el usuario, sin condicionar la máquina de estados SDD.

#### Scenario: Invocación autónoma de comandos review en la CLI
- **GIVEN** el binario `axiom` compilado.
- **WHEN** el usuario ejecuta `axiom review --help` o cualquier subcomando de revisión (`axiom review mode`, `axiom review start`).
- **THEN** la CLI debe responder correctamente ofreciendo las opciones de auditoría de código sin alterar el estado de las fases SDD del workspace.

---

### Requirement: Resiliencia en Decodificación de Rutas Windows en Salidas JSON de SDD (REQ-18.3)
Las aserciones y consumidores de la salida JSON de `sdd status` deben decodificar adecuadamente las estructuras JSON en tipos nativos Go en lugar de realizar comparaciones literales de cadenas con subdirectorios de Windows.

#### Scenario: Validación de rutas permitidas bajo Windows
- **GIVEN** un entorno de ejecución Windows con rutas que contienen separadores de barra invertida (`\`).
- **WHEN** un comando de prueba o consumidor externo valida las rutas de edición concedidas (`actionContext.allowedEditRoots`) en la salida JSON de `RunSDDStatus`.
- **THEN** la validación debe realizarse mediante deserialización `json.Unmarshal` en `StatusV2Projection`, tolerando el escape JSON de backslashes sin fallos de aserción.

---

### Requirement: Aislamiento del Directorio de Trabajo (CWD) en la Gestión de Artefactos (REQ-18.4)
La determinación de rutas de configuración de agentes y perfiles (`persona.json`, `skills/`) debe resolverse contra sus directorios raíz canónicos (`home` o `workspaceRoot`) sin depender del directorio de trabajo (`CWD`) actual desde el que se invoca `axiom`.

#### Scenario: Sincronización de agentes desde un subdirectorio anidado
- **GIVEN** un workspace válido de Axiom y un proceso CLI ejecutándose en un subdirectorio profundo.
- **WHEN** se ejecuta una operación de sincronización o comprobación de rutas de agentes (`axiom sync`).
- **THEN** los ficheros de configuración global y de workspace deben escribirse en sus ubicaciones canónicas absolutas sin crear artefactos huérfanos relativos al CWD.

---

### Requirement: Saneamiento del Preset Predeterminado de Skills (REQ-18.5)
El catálogo de skills distribuido en el preset básico de Axiom (`foundationSkills`) debe contener exclusivamente herramientas de productividad para usuarios finales, segregando las skills de colaboración interna del repositorio (`branch-pr`, `issue-creation`, `systemic-issue-triage`, `rdd-defect-workflow`, etc.) al catálogo elegible avanzado.

#### Scenario: Inspección del preset básico de skills
- **GIVEN** el paquete `internal/components/skills`.
- **WHEN** se consulta el listado de skills pertenecientes al preset por defecto.
- **THEN** el preset básico debe incluir únicamente las skills fundacionales del producto.
- **AND** no debe incluir skills destinadas al flujo interno de contribución del repositorio.

---

### Requirement: Resiliencia de Protocolo Engram ante Ambiguous Project (REQ-18.6)
El protocolo y directrices de integración con Engram MCP deben documentar y contemplar el tratamiento ordenado del estado `ambiguous_project` en el handshake de inicialización de sesión, guiando la recuperación sin bloqueos.

#### Scenario: Manejo de ambigüedad de proyecto en handshake de sesión Engram
- **GIVEN** un entorno con múltiples workspaces registrados.
- **WHEN** el agente inicia una sesión con Engram MCP y se devuelve un código o indicación de proyecto ambiguo.
- **THEN** el protocolo documentado debe instruir la resolución unívoca especificando explícitamente el identificador de proyecto antes de reintentar.

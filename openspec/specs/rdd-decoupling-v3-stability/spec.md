# Especificación Viva: Desacoplamiento de RDD y Parches de Estabilidad Upstream v3

> **Dominio:** `rdd-decoupling-v3-stability`  
> **Versión Canónica:** 1.0.0  
> **Estado:** Vigente  
> **Idioma:** Español (Castellano peninsular)  

---

## 1. Contexto y Propósito

Esta especificación consolida la arquitectura canónica de Axiom respecto a la independencia del ciclo de vida Spec-Driven Development (SDD) frente a Review-Driven Development (RDD), así como la absorción de correcciones críticas de estabilidad provenientes de Gentle-AI v3:

1. **Desacoplamiento Estructural de RDD:** La proyección de estado SDD (`sdd status`) no contiene ofertas ni dependencias forzadas con transacciones de revisión. El avance hacia el archivado (`archive: ready`) está desacoplado de la participación de RDD y depende únicamente de la compleción de tareas y el veredicto favorable en `verify-report.md`.
2. **Independencia de Herramientas de Revisión:** Las utilidades de revisión de código (`axiom review ...`) son herramientas opt-in independientes gobernadas por el usuario que no interfieren en la máquina de estados SDD.
3. **Resiliencia en Windows:** Las aserciones y procesamiento de respuestas JSON de la CLI bajo Windows emplean decodificación estructurada en tipos Go nativos para evitar fallos por barras invertidas escapadas.
4. **Aislamiento de CWD en Gestión de Artefactos:** Los artefactos de agentes y persona se resuelven contra directorios canónicos absolutos (`homeDir`, `componentInjectionDir`) sin depender del directorio de trabajo actual.
5. **Saneamiento de Presets:** Los presets de usuario general (`selectableFoundationSkills`) contienen exclusivamente skills de producto, excluyendo las skills de flujos internos de colaboración (`contributorSkills`).
6. **Resiliencia de Engram:** Protocolo unificado de recuperación determinista ante el estado `ambiguous_project`.

---

## 2. Requerimientos Canónicos y Escenarios BDD

### Requirement: Desacoplamiento de RDD en la Proyección de Estado SDD (REQ-18.1)
El motor de estados en `internal/sddstatus` DEBE omitir de forma limpia el bloque `reviewOffer` y llamadas acopladas a transacciones de revisión. La preparación para el archivado (`archive: ready`) DEBE evaluarse de forma independiente a la presencia de revisiones de código.

#### Scenario: Proyección de estado SDD tras verificación exitosa sin reviewOffer forzado
- **DADO** un incremento en fase de verificación con tareas completadas al 100% y `verify-report.md` con veredicto PASS
- **CUANDO** se ejecuta la proyección de estado `axiom sdd status [cambio] --json`
- **ENTONCES** el documento JSON omitirá la clave `reviewOffer`
- **Y** el campo `dependencies.archive` se evaluará como `ready`

---

### Requirement: Independencia Operativa de las Herramientas RDD (REQ-18.2)
Las herramientas de revisión (`axiom review ...`) DEBEN operar como comandos opt-in independientes gobernados por el usuario, sin condicionar la máquina de estados SDD.

#### Scenario: Invocación autónoma de comandos review en la CLI
- **DADO** el binario `axiom`
- **CUANDO** el usuario ejecuta comandos bajo `axiom review`
- **ENTONCES** la CLI ejecutará las operaciones de revisión sin alterar las fases SDD del workspace

---

### Requirement: Resiliencia en Decodificación de Rutas Windows en Salidas JSON de SDD (REQ-18.3)
Los consumidores y aserciones de la salida JSON de `sdd status` DEBEN decodificar estructuradamente en tipos Go nativos (`StatusV2Projection`) en lugar de hacer matching textual simple de rutas en Windows.

#### Scenario: Validación de rutas permitidas bajo Windows
- **DADO** un entorno de ejecución Windows con rutas que contienen separadores de barra invertida (`\`)
- **CUANDO** se valida la salida JSON de `RunSDDStatus`
- **ENTONCES** la validación se realiza mediante `json.Unmarshal`, tolerando el escape de backslashes sin fallos de aserción

---

### Requirement: Aislamiento del Directorio de Trabajo (CWD) en la Gestión de Artefactos (REQ-18.4)
La resolución de rutas de perfiles de agentes y persona DEBE realizarse contra sus directorios raíz canónicos (`homeDir` o `workspaceRoot`) sin depender del CWD.

#### Scenario: Sincronización de agentes desde un subdirectorio anidado
- **DADO** un workspace de Axiom e invocación desde un subdirectorio
- **CUANDO** se ejecuta una sincronización (`axiom sync`)
- **ENTONCES** los archivos se escriben en sus ubicaciones canónicas absolutas sin crear artefactos relativos al CWD

---

### Requirement: Saneamiento del Preset Predeterminado de Skills (REQ-18.5)
El catálogo de skills distribuido en el preset básico de Axiom DEBE contener únicamente skills de producto para usuarios, segregando las skills de colaboración interna del repositorio (`contributorSkills`).

#### Scenario: Inspección del preset básico de skills
- **DADO** el catálogo de skills en `internal/components/skills`
- **CUANDO** se consulta el listado del preset por defecto
- **ENTONCES** contiene únicamente skills fundacionales de producto y excluye las de contribución interna

---

### Requirement: Resiliencia de Protocolo Engram ante Ambiguous Project (REQ-18.6)
El protocolo y directrices de integración con Engram MCP DEBEN documentar y contemplar el tratamiento ordenado del estado `ambiguous_project` en el apretón de manos inicial.

#### Scenario: Manejo de ambigüedad de proyecto en handshake de sesión Engram
- **DADO** un entorno multi-proyecto con Engram MCP
- **CUANDO** el agente detecta una indicación de proyecto ambiguo
- **ENTONCES** el protocolo instruye la especificación unívoca del identificador de proyecto mediante `--project=<nombre>` antes de reintentar

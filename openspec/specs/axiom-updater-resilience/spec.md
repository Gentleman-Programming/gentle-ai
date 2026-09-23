# Especificación Viva: Resiliencia y Gobernanza del Actualizador Autónomo de Axiom

> **Dominio:** `axiom-updater-resilience`  
> **Versión Canónica:** 1.0.0  
> **Estado:** Vigente  
> **Idioma:** Español (Castellano peninsular)

---

## 1. Capacidad: `axiom-updater-resilience`

Estrategias y salvaguardas de actualización del binario `axiom` para entornos Windows, macOS y Linux: resolución segura del destino de instalación, vía resiliente `sourceBuildUpgrade`, encadenamiento `upgrade` ➔ `sync` en Web UI y TUI, y preservación del contrato CLI solo-binario.

### Requirement: Ejecución segura de la actualización en Windows (REQ-22.1)

El sistema DEBE aplicar, en entorno Windows, una estrategia de actualización de la herramienta del fork que no falle por discrepancia de ruta de módulo en `go.mod` y que reemplace el binario de forma segura y transparente. Cuando ninguna estrategia automatizada pueda garantizar esas dos condiciones, el sistema DEBE degradar a una instrucción de actualización manual accionable con `ManualFallbackError` y NO DEBE dejar el binario a medias ni duplicar binarios en PATH.

#### Scenario: Degradación manual segura ante destinos disjuntos
- **DADO** un entorno Windows donde el ejecutable activo en PATH difiere del destino resuelto por `go env GOBIN`/`GOPATH`
- **CUANDO** se intenta una auto-actualización
- **ENTONCES** la actualización se rehúsa antes de escribir
- **Y** se emite una instrucción manual precisa con comandos para el usuario

---

### Requirement: Predicado de identidad del módulo para Go install (REQ-22.2)

Las instrucciones de instalación y las comprobaciones de compatibilidad DEBEN derivarse de la estructura `ToolInfo` asociada al binario (`GoModulePath`, `GoImportPath`), reconociendo la identidad del fork mediante el predicado `IsSelfToolName`/`IsSelfTool`. La función `GoInstallResolvable()` DEBE validar que la ruta declarada sea alcanzable mediante `go install` estándar antes de seleccionar esa estrategia.

#### Scenario: Ruteo condicionado por resolubilidad de módulo
- **DADO** que `GoInstallResolvable()` retorna falso para el binario `axiom`
- **CUANDO** se resuelve el método de actualización en Windows
- **ENTONCES** el actualizador selecciona la estrategia de construcción desde código fuente (`InstallSourceBuild`) o degradación manual
- **Y** no invoca `go install` contra un import path no coincidente con `go.mod`

---

### Requirement: Salvaguardas de actualización ancladas a la identidad del fork (REQ-22.3)

Todas las salvaguardas de comprobación previa de binarios en ejecución, renombrado atómico temporal y validación de procedencia DEBEN operar sobre la identidad del fork (`axiom`) y no sobre literales heredados de upstream (`gentle-ai`).

#### Scenario: Verificación de binario en ejecución
- **DADO** una actualización en caliente de `axiom.exe`
- **CUANDO** se ejecuta el reemplazo
- **ENTONCES** se verifica la firma y el proceso bajo el nombre `axiom`
- **Y** se asegura la terminación limpia o renombramiento transitorio

---

### Requirement: Endpoint Web UI encadena upgrade ➔ sync con reporte consolidado (REQ-22.4)

El endpoint `POST /api/ecosystem/upgrade` del Dashboard Web DEBE ejecutar secuencialmente la fase de actualización de binarios (`upgrade`) seguida de la fase de sincronización de reglas y configuraciones (`sync`), retornando un DTO consolidado con el desglose por fases (`phases.upgrade`, `phases.sync`). Si la fase `upgrade` requiere reinicio del proceso (`restart_required: true`), la fase `sync` DEBE omitirse limpiamente (`skipped_reason: "restart-required"`).

#### Scenario: Secuencia completada en el Dashboard
- **DADO** una petición `POST /api/ecosystem/upgrade` desde la Web UI
- **CUANDO** no se requiere reinicio inmediato del servidor
- **ENTONCES** se ejecuta `upgrade` y posteriormente `sync`
- **Y** la respuesta 200 OK contiene el reporte de ambas fases

---

### Requirement: No-regresión del verbo CLI axiom upgrade (REQ-22.5)

El comando de terminal `axiom upgrade` DEBE mantenerse estrictamente como una operación **solo-binario**; NO DEBE encadenar automáticamente `sync` ni `install`. La composición de `upgrade` ➔ `sync` es exclusiva de las superficies reactivas interactivas (Web UI y TUI).

#### Scenario: Ejecución de axiom upgrade desde consola
- **DADO** la invocación de `axiom upgrade` en CLI
- **CUANDO** se completa la actualización de herramientas
- **ENTONCES** solo se actualizan los binarios
- **Y** no se mutan configuraciones ni se disparan sincronizaciones de agentes

---

### Requirement: Paridad TUI: encadenamiento y protección de reinicio (REQ-22.6)

La vista combinada de actualización y sincronización de la TUI interactiva (`ScreenUpgradeSync`) DEBE ejecutar la secuencia `upgrade` ➔ `sync`, mostrando el avance en pantalla y respetando la precondición de reinicio si el binario activo fue modificado.

#### Scenario: Vista de actualización en TUI
- **DADO** la selección de actualización en la TUI
- **CUANDO** finaliza la descarga del nuevo binario
- **ENTONCES** informa al usuario del estado y ejecuta sync si es seguro

---

### Requirement: Identidad visible del fork en vistas de actualización (REQ-22.7)

Todos los mensajes de confirmación, cabeceras, títulos y diálogos en TUI y Web UI DEBEN referirse exclusivamente a la identidad de `axiom`.

#### Scenario: Textos de interfaz limpios
- **DADO** la pantalla `ScreenUpgradeSync`
- **CUANDO** se renderizan los diálogos de progreso
- **ENTONCES** todos los textos nombran `axiom`

---

### Requirement: Versión de build visible y consistente (REQ-22.8)

El símbolo de versión en `cmd/axiom/main.go` DEBE declararse como variable mutable (`var version = "v0.1.0"`), permitiendo que el linker (`-X main.version=...`) en los flujos de CI y GoReleaser inyecte el tag exacto de la versión compilada.

#### Scenario: Inyección de versión en tiempo de enlace
- **DADO** una compilación con `-ldflags="-X main.version=v3.5.0"`
- **CUANDO** se ejecuta `axiom --version`
- **ENTONCES** la salida refleja `v3.5.0`

---

### Requirement: Registro durable de la versión upstream de referencia (REQ-22.9)

El archivo de estado de usuario (`~/.axiom/state.json`) DEBE mantener la clave de auditoría `upstream_version` inicializada con el techo de referencia (`"3.4.0"`), sin ser sobreescrita ni consumida automáticamente por rutas de ejecución.

#### Scenario: Conservación de la versión upstream de referencia
- **DADO** la inicialización de estado de Axiom
- **CUANDO** se consulta `state.json`
- **ENTONCES** el registro contiene `upstream_version: "3.4.0"`

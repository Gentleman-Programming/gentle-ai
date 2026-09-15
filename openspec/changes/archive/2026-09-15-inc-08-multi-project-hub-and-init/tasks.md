# Tareas: Hub Multi-Proyecto, Selector Dinámico en Dashboard Web y CLI axiom init (INC-08)

## Fase 1: Dominio del Registro Central y Detección (`internal/hub/`)

- [x] T-01 Crear `internal/hub/types.go`: Modelos de datos para `WorkspaceRecord`, `HubConfig`, `TechDetection`, `InitOptions` e `InitResult`.
- [x] T-02 Crear `internal/hub/detector.go`: Detector heurístico de tecnologías (Go, .NET/C#, Node/TypeScript, Python, Rust).
- [x] T-03 Crear `internal/hub/manager.go`: Gestor con persistencia atómica en `~/.axiom/workspaces.json`, operaciones de registro (`Register`), desregistro (`Unregister`), listado (`List`), conmutación (`SetActive`) y consulta (`GetActive`).
- [x] T-04 Crear `internal/hub/init.go`: Inicializador de workspaces (`Init`) que genera `axiom.yaml`, carpetas base (`.axiom/inbox/skills/`, `openspec/specs/`, `openspec/changes/`) y vincula con el Hub.
- [x] T-05 Crear `internal/hub/hub_test.go`: Suite completa de pruebas unitarias para persistencia, detección, inicialización y concurrencia.

## Fase 2: Integración Dinámica en Dashboard Web (`internal/dashboard/`)

- [x] T-06 Actualizar `internal/dashboard/service.go`: Incorporar `hub.Manager` y habilitar conmutación dinámica del workspace activo (`SwitchWorkspace`) en tiempo de ejecución.
- [x] T-07 Actualizar `internal/dashboard/server.go`: Exponer endpoints REST `/api/projects` (GET), `/api/projects/switch` (POST), `/api/projects/add` (POST) y `/api/projects/init` (POST).
- [x] T-08 Actualizar `internal/dashboard/assets/` (`index.html`, `style.css`, `app.js`):
  - Añadir selector desplegable de proyectos en el Navbar superior.
  - Añadir modal "+ Añadir Proyecto".
  - Añadir pantalla de bienvenida "Zero-Config" cuando una carpeta no tiene `axiom.yaml`, con botón de inicialización reactivo.
- [x] T-09 Actualizar `internal/dashboard/dashboard_test.go`: Tests unitarios para los nuevos endpoints de proyectos.

## Fase 3: Integración CLI (`cmd/axiom/main.go`)

- [x] T-10 Implementar subcomando `axiom init`: Generación de `axiom.yaml`, estructura inicial y registro en Hub.
- [x] T-11 Implementar subcomando `axiom project`: `axiom project list`, `switch`, `add` y `remove`.
- [x] T-12 Actualizar subcomando `axiom ui`: Conectar con el Hub de proyectos para resolver el workspace activo o abrir en modo Hub general.

## Fase 4: Verificación, Pruebas y Cierre SDD

- [x] T-13 Ejecutar pruebas unitarias de todos los paquetes (`go test ./internal/...`) y verificar 100% de éxito sin regresiones.
- [x] T-14 Compilar e instalar globalmente el nuevo binario de Axiom (`go install ./cmd/axiom` y `go build -o axiom.exe ./cmd/axiom`).
- [x] T-15 Verificar en vivo el flujo multi-proyecto (ejecutar `axiom ui` con selector de proyectos y probar `axiom init`).
- [x] T-16 Generar reportes de verificación y archivo: `verify-report.md`, `archive-report.md` y promover la especificación viva a `openspec/specs/multi-project-hub/spec.md`.

# ODD: Buscador y Explorador Local de Carpetas en la UI de Axiom

> **Documento vivo ODD.** Fichero autoritativo: `odd/tasks/local-folder-picker-ui.md`.  
> Espejo de recuperación en Engram: topic `odd/local-folder-picker-ui/tasks`, proyecto `axiom`.

## Objetivo

Implementar un buscador y selector interactivo de carpetas locales en el Dashboard Web (`axiom ui`) para eliminar la fricción de tener que escribir o copiar manualmente rutas absolutas del disco:
1. **Modal de Vincular / Inicializar Proyecto (`#modal-add-project`):**
   - Incorporar botón «📁 Examinar...» junto al campo «Ruta absoluta del proyecto en disco» (`#input-project-path`).
   - Al seleccionar una carpeta válida, rellenar la ruta y autocompletar automáticamente el «Nombre descriptivo» (`#input-project-name`) a partir del nombre base de la carpeta si estaba vacío.
2. **Constructor Dinámico de Roles:**
   - Incorporar botón «📁» junto al campo «Rutas / Repositorios» (`.role-repos-input`) en cada fila de rol para explorar y seleccionar subcarpetas o repositorios adicionales.
3. **Acceso Rápido desde la Cabecera:**
   - Permitir invocar el selector de carpetas o vincular un nuevo proyecto de forma fluida.
4. **Backend de Exploración del Sistema de Archivos Local (`/api/fs/directories`):**
   - Endpoint HTTP seguro que lista unidades de disco (en Windows `C:\`, `D:\`), ruta de usuario (`~`), ruta del workspace y subdirectorios del directorio seleccionado.
   - Navegación ascendente (`..`), breadcrumbs interactivos y filtrado en tiempo real.
   - Endpoint auxiliar opcional (`POST /api/fs/native-picker`) para invocar el selector nativo del sistema operativo si el usuario lo prefiere.
5. **Componente Visual Reutilizable (`FolderPicker`):**
   - Modal accesible y estilizado con el tema oscuro de Axiom (`z-index` adecuado sobre otros modales).
   - Barra de navegación con breadcrumbs cliqueables, botón de subir nivel, input de ruta manual con salto directo, accesos directos (Inicio, Workspace, Unidades), filtro en tiempo real y lista desplazable de carpetas.

---

## Tareas

- [x] **T1 · Backend: API de Exploración del Sistema de Archivos (`internal/dashboard/`)**
  - Implementar DTOs `FileSystemBrowseResult` y `FileSystemDirectoryItem` en `internal/dashboard/types.go` (o `fs_types.go`).
  - Implementar `BrowseDirectories(path string)` en `service.go` / `fs_service.go`, con soporte multiplataforma para unidades (Windows API / discos Unix).
  - Registrar ruta `GET /api/fs/directories` en `server.go` y su handler correspondiente.
  - Implementar tests unitarios completos en `internal/dashboard/`.
- [x] **T2 · Frontend: Componente Modal Reutilizable de Selección de Carpetas (`assets/index.html`, `app.js`, `style.css`)**
  - Añadir estructura HTML del modal `#modal-folder-picker` en `index.html`.
  - Diseñar estilos CSS para navegación, breadcrumbs, chips de unidades, lista de carpetas y pie con selección activa en `style.css`.
  - Crear lógica JavaScript modular para abrir el selector (`openFolderPicker({ initialPath, onSelect })`), cargar directorios, navegar por breadcrumbs, filtrar en tiempo real y confirmar selección.
- [x] **T3 · Integración en Modales y Entradas de Rutas**
  - Integrar botón «📁 Examinar...» en `#modal-add-project` para `#input-project-path`.
  - Integrar autocompletado del nombre del proyecto en `#input-project-name`.
  - Integrar botón «📁» en las filas de roles (`.role-repos-input`).
  - Integrar botón «📁 Explorar...» en la cabecera junto al selector de proyectos.
- [x] **T4 · Verificación y Pruebas Automatizadas**
  - Ejecutar suite de pruebas de dashboard (`go test ./internal/dashboard/... -v`).
  - Verificar respuesta de los endpoints y renderizado de assets embebidos.
- [x] **T5 · Sincronización en Engram y Compilación**
  - Sincronizar el documento de tarea con Engram MCP y compilar binario.

---

## Verificación Ejecutable
- `go test ./internal/dashboard/... -run "TestBrowseDirectories|TestFS|TestEmbeddedAssets" -v` -> PASS (0.255s)
- `go test ./internal/dashboard/... -v` -> PASS (36.578s, todas las pruebas existentes y nuevas en verde)
- `go vet ./...` -> PASS (código estático limpio sin alertas)
- `go build -o ./axiom-test.exe ./cmd/axiom` -> PASS (compilación limpia del ejecutable)


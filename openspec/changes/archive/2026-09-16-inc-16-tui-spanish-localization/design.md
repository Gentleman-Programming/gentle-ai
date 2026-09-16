# Diseño Técnico: Localización Integral en Castellano de la TUI Bubbletea y Desacoplamiento de Advisories (INC-16)

## Contexto y Arquitectura de la TUI

La TUI interactiva de Axiom está construida con el framework `charmbracelet/bubbletea` y estilos de `lipgloss`. El modelo principal (`internal/tui/model.go`) gestiona la máquina de estados, el enrutamiento de pantallas (`Screen`) y el cursor de selección (`m.Cursor`).

Un principio arquitectónico fundamental del despachador en `model.go` es:
- **Despacho por índice:** Las transiciones de pantalla del menú de bienvenida se realizan evaluando el índice numérico de `m.Cursor` (`0, 1, 2...`), NO comparando las cadenas de texto visibles.
- **Invarianza de navegación:** Mantener el orden y número de opciones de `WelcomeOptions` garantiza que la máquina de estados permanezca completamente determinista y funcional.

---

## Decisiones de Diseño

### 1. Desacoplamiento Definitivo de Avisos Upstream (`internal/update/advisory.go`)
- **Estado actual:** `advisoryURL` apuntaba a `Gentleman-Programming/gentle-ai/releases/download/advisory/advisory.json`.
- **Diseño:** Apunta a `https://github.com/IGutierrezZ/axiom/releases/download/advisory/advisory.json`.
- **Comportamiento fail-open:** Al devolver 404 o estar vacío, `advisory.Message` y `advisory.URL` son cadenas vacías, con lo que `welcomeAdvisoryLines` retorna `nil` y no se renderiza ninguna línea de advertencia o release ajena en la cabecera.

### 2. Localización del Menú Principal (`internal/tui/screens/welcome.go`)
Mapeo de opciones en `WelcomeOptions`:
- `"Start installation"` ➔ `"Iniciar instalación"`
- `"Upgrade tools"` ➔ `"Actualizar herramientas"`
  - `"Upgrade tools ★"` ➔ `"Actualizar herramientas ★"`
  - `"Upgrade tools (up to date)"` ➔ `"Actualizar herramientas (al día)"`
- `"Sync configs"` ➔ `"Sincronizar configuraciones"`
- `"Upgrade + Sync"` ➔ `"Actualizar y sincronizar"`
- `"Configure models"` ➔ `"Configurar modelos"`
- `"Create your own Agent"` ➔ `"Crear agente personalizado"`
  - Con `!hasEngines`: `"Crear agente personalizado (sin motores)"`
- `"OpenCode Community Plugins"` ➔ `"Plugins comunitarios de OpenCode"`
- `"Uninstall OpenCode Plugin"` ➔ `"Desinstalar plugin de OpenCode"`
- `"OpenCode SDD Profiles"` ➔ `"Perfiles SDD de OpenCode"`
  - Con contador: `"Perfiles SDD de OpenCode (%d)"`
- `"Manage backups"` ➔ `"Gestionar respaldos"`
- `"Reset review store"` ➔ `"Reiniciar almacén de revisiones"`
- `"Receipt-Driven Development"` ➔ `"Revisión formal RDD"`
- `"Managed uninstall"` ➔ `"Desinstalación gestionada"`
- `"Community Tools/Plugins"` ➔ `"Herramientas y plugins comunitarios"`
- `"Quit"` ➔ `"Salir"`

Microtextos y ayudas en `welcome.go`:
- Título: `"Menu"` ➔ `"Menú"`
- Barra de navegación: `"j/k: navigate • enter: select • q: quit"` ➔ `"j/k: navegar • enter: seleccionar • q: salir"`
- Acciones mínimas: `"Start installation"` ➔ `"Iniciar instalación"`, `"Go"` ➔ `"Ir"`
- Avisos: `"Advisory: "` ➔ `"Aviso: "`, `"Latest release: "` ➔ `"Última versión: "`
- Scroll de avisos: `"PgUp/PgDn: scroll  •  lines %d-%d/%d"` ➔ `"RePág/AvPág: desplazar  •  líneas %d-%d/%d"`

### 3. Localización de Submenús y Pantallas de Configuración
- **`screens/model_config.go`:**
  - Título: `"Model Configuration"` ➔ `"Configuración de Modelos"`
  - Subtexto: `"Choose which AI model to configure:"` ➔ `"Elige qué motor de IA deseas configurar:"`
  - Opciones:
    - `"Configure Claude models"` ➔ `"Configurar modelos de Claude"`
    - `"Configure OpenCode models"` ➔ `"Configurar modelos de OpenCode"`
    - `"Configure Kiro models"` ➔ `"Configurar modelos de Kiro"`
    - `"Configure Codex models"` ➔ `"Configurar modelos de Codex"`
    - `"Back"` ➔ `"Volver"`
  - Barra de ayuda: `"j/k: navigate • enter: select • esc: back • q: quit"` ➔ `"j/k: navegar • enter: seleccionar • esc: volver • q: salir"`

- **`screens/backups.go`:**
  - Título: `"Backup Management"` ➔ `"Gestión de Respaldos"`
  - Estados: `"No backups found yet."` ➔ `"No se encontraron respaldos."`
  - Botón: `"Back"` ➔ `"Volver"`
  - Scroll: `"  ↑ more"` ➔ `"  ↑ más"`, `"  ↓ more"` ➔ `"  ↓ más"`
  - Barra de ayuda: `"j/k: navigate • enter: restore • r: rename • d: delete • p: pin/unpin • esc: back"` ➔ `"j/k: navegar • enter: restaurar • r: renombrar • d: eliminar • p: fijar/desfijar • esc: volver"`
  - Confirmación de restauración: `"Restore Backup"`, `"Are you sure..."`, botones `"Restaurar"`, `"Cancelar"`.
  - Confirmación de eliminación: `"Delete Backup"`, botones `"Eliminar"`, `"Cancelar"`.
  - Renombrar respaldo: `"Rename Backup"`, `"Current description:"` ➔ `"Descripción actual:"`, `"New description:"` ➔ `"Nueva descripción:"`.

- **`screens/detection.go`:**
  - Título: `"Detección del Sistema"`, opciones `["Continuar", "Volver"]`.
  - Indicadores: `"Sí"`, `"No"`, `"encontrado"`, `"no encontrado"`, `"presente"`, `"ausente"`, `"requerido"`, `"opcional"`.

- **`screens/preset.go`:**
  - Título: `"Seleccionar Preset del Ecosistema"`
  - Presets: `"Solo Memoria"`, `"Pila de Desarrollo"`, `"Pila de Desarrollo + Pulido"`, `"Personalizado"`.
  - Descripciones en castellano profesional.

- **`screens/persona.go`:**
  - Título: `"Elige tu Persona"`
  - Subtexto: `"Tu asistente de ingeniería que enseña antes de resolver."`
  - Descripciones de personas en castellano.

- **`screens/common.go`:**
  - `"Please wait..."` ➔ `"Por favor, espera..."`

### 4. Ajuste de Pruebas Unitarias
- Actualizar `internal/tui/screens/welcome_test.go` y `internal/tui/screens/welcome_internal_test.go` para validar las nuevas cadenas en español.
- Actualizar `internal/tui/model_test.go` en los puntos donde se evalúan cadenas de pantalla inicial (`"Start installation"` ➔ `"Iniciar instalación"`, `"Quit"` ➔ `"Salir"`, etc.).
- Actualizar `internal/tui/screens/backups_test.go`, `model_config_test.go`, `preset_test.go`, etc.

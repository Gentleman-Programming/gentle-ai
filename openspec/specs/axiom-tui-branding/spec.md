<!-- Especificación Viva generada a partir de '2026-09-16-inc-14-axiom-tui-branding-and-cli-cutover' -->

# Especificación de Requerimientos: Unificación de TUI Bubbletea, Comandos de Ecosistema en CLI axiom y Pasarela de gentle-ai (INC-14)

## Propósito

Definir de forma rigurosa los requerimientos funcionales y escenarios BDD para la consolidación de la TUI interactiva de Axiom, el enrutamiento de comandos de instalación y sincronización en la CLI `axiom`, y la pasarela de compatibilidad de `gentle-ai`.

---

## 1. Capacidad: `axiom-tui-branding`

Renovación visual de la interfaz de terminal interactiva (TUI) con la identidad de Axiom.

### Requirement: Logotipo ASCII y Lema de Axiom en TUI (REQ-14.1)
La pantalla de bienvenida de la TUI de Bubbletea DEBE renderizar un logotipo tipográfico en arte ASCII de **AXIOM** y el lema canónico de la plataforma, eliminando por completo cualquier silueta o alusión a la rosa o denominación de Gentle AI.

#### Scenario: Renderizado del logotipo tipográfico de Axiom
- **DADO** la función `RenderLogo()` en `internal/tui/styles/logo.go`
- **CUANDO** se invoca para componer la cabecera visual
- **ENTONCES** genera una representación gráfica de las letras "AXIOM"
- **Y** no contiene las líneas ASCII de la silueta de la rosa de Gentle AI
- **Y** aplica los estilos de color de gradiente configurados en Lipgloss

#### Scenario: Lema institucional de Axiom en pantalla de bienvenida
- **DADO** la función `Tagline(version)` en `internal/tui/styles/styles.go`
- **CUANDO** se formatea el subtítulo para la versión `v0.1.0`
- **ENTONCES** retorna `"Axiom v0.1.0 — Plataforma SDD Multi-Rol y Multi-Repositorio"`
- **Y** no contiene el prefijo `"Gentle-AI"`

---

## 2. Capacidad: `axiom-cli-ecosystem-commands`

Integración de la TUI y los comandos de ciclo de vida del ecosistema de agentes en la CLI soberana `axiom`.

### Requirement: Lanzamiento de TUI interactiva por defecto y subcomando axiom tui (REQ-14.2)
Al invocar `axiom` sin argumentos, si la sesión cuenta con terminal interactivo (TTY) tanto en entrada estándar como en salida estándar, DEBE iniciarse la TUI interactiva. Si no hay TTY disponible o se solicita explícitamente `--help`, DEBE imprimirse la ayuda textual. Además, el subcomando explícito `axiom tui` DEBE lanzar siempre la TUI interactiva.

#### Scenario: Ejecución de axiom sin argumentos en sesión interactiva TTY
- **DADO** un entorno con terminal interactivo TTY (stdin y stdout son TTY)
- **CUANDO** el usuario ejecuta `axiom` sin argumentos adicionales
- **ENTONCES** se inicia el programa Bubbletea de la TUI de Axiom

#### Scenario: Ejecución de axiom sin argumentos en entorno no interactivo (CI o tubería)
- **DADO** un entorno donde stdin o stdout no son un terminal TTY
- **CUANDO** se invoca `axiom` sin argumentos
- **ENTONCES** el comando imprime la ayuda textual en stdout y finaliza con código `0`

#### Scenario: Lanzamiento explícito mediante axiom tui
- **DADO** un terminal interactivo
- **CUANDO** el usuario ejecuta `axiom tui`
- **ENTONCES** se lanza la TUI de Axiom independientemente de los argumentos base

### Requirement: Subcomandos de gestión de herramientas y agentes en CLI axiom (REQ-14.3)
El binario `axiom` DEBE soportar nativamente los subcomandos de gestión de herramientas: `install`, `sync`, `upgrade`, `doctor`, `backup`, `restore` y `uninstall`, delegando la ejecución a los módulos correspondientes del runtime.

#### Scenario: Ayuda de install desde axiom
- **DADO** el binario `axiom`
- **CUANDO** el usuario ejecuta `axiom install --help`
- **ENTONCES** se muestra la guía de opciones de instalación de agentes con código `0`

#### Scenario: Sincronización de agentes mediante axiom sync
- **DADO** agentes configurados en el entorno del usuario
- **CUANDO** el usuario ejecuta `axiom sync [flags]`
- **ENTONCES** se ejecutan las rutinas de sincronización de contratos y skills en los agentes soportados

#### Scenario: Diagnóstico del ecosistema mediante axiom doctor
- **DADO** un entorno de trabajo con herramientas de desarrollo
- **CUANDO** el usuario ejecuta `axiom doctor`
- **ENTONCES** se evalúa la salud de los agentes, dependencias y rutas de configuración del usuario

---

## 3. Capacidad: `gentle-ai-compat-wrapper`

Pasarela de compatibilidad y transición no disruptiva para usuarios y scripts existentes.

### Requirement: Pasarela de compatibilidad y deprecación de gentle-ai (REQ-14.4)
El binario `gentle-ai` DEBE actuar como un envoltorio ligero que emite una advertencia de deprecación en `stderr` recomendando migrar al comando `axiom`, mientras delega fielmente la ejecución con todos los argumentos al motor unificado.

#### Scenario: Advertencia informativa de deprecación al invocar gentle-ai
- **DADO** una llamada al binario `gentle-ai --version`
- **CUANDO** se ejecuta el comando
- **ENTONCES** en `stderr` se emite un mensaje indicando que `gentle-ai` está deprecado y unificado en `axiom`
- **Y** en `stdout` se emite la salida esperada del comando ejecutado
- **Y** el código de salida coincide con el resultado de la operación

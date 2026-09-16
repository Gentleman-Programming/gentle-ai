# Especificación Viva: Localización Integral al Castellano y Desacoplamiento de Avisos en la TUI (Axiom)

## 1. Capacidad: `tui-spanish-localization`

Garantiza que la interfaz gráfica interactiva de terminal (TUI Bubbletea) de Axiom opere con identidad propia, limpia de avisos o dependencias de versiones externas, y que todos sus menús, submenús, diálogos, estados y atajos de navegación se presenten estrictamente en español (castellano peninsular).

---

### Requirement: Desacoplamiento de Comunicados y Avisos Remotos Upstream (REQ-16.1)
El sistema NO debe consultar ni desplegar en la cabecera de la TUI ningún comunicado, URL de descarga o advertencia perteneciente a Gentle AI.

#### Scenario: Inicio limpio de la TUI sin avisos ajenos
- **DADO QUE** el usuario ejecuta `axiom tui` o `axiom` en un terminal interactivo
- **CUANDO** la pantalla de bienvenida se inicializa
- **ENTONCES** no debe aparecer ninguna línea con `Latest release: https://github.com/Gentleman-Programming/gentle-ai/...`
- **Y** no debe aparecer ningún mensaje de `Advisory: Gentle AI v3.0.0...`
- **Y** el encabezado debe mostrar únicamente el banner de Axiom y la versión local del sistema.

#### Scenario: Consulta de avisos en repo de Axiom con fail-open
- **DADO QUE** `advisoryURL` apunta a `https://github.com/IGutierrezZ/axiom/releases/download/advisory/advisory.json`
- **CUANDO** el recurso remoto retorne un código 404 Not Found o no esté disponible
- **ENTONCES** la TUI debe continuar su flujo normal de apertura sin error ni latencia perceptible.

---

### Requirement: Localización al Castellano del Menú de Bienvenida (REQ-16.2)
El menú principal de bienvenida (`WelcomeOptions`) y sus microtextos auxiliares deben presentarse íntegramente en español (castellano peninsular).

#### Scenario: Visualización de opciones en español
- **DADO QUE** el usuario se encuentra en la pantalla de bienvenida
- **CUANDO** se renderiza la lista de opciones
- **ENTONCES** las etiquetas deben ser:
  - "Iniciar instalación"
  - "Actualizar herramientas" / "Actualizar herramientas (al día)" / "Actualizar herramientas ★"
  - "Sincronizar configuraciones"
  - "Actualizar y sincronizar"
  - "Configurar modelos"
  - "Crear agente personalizado"
  - "Plugins comunitarios de OpenCode"
  - "Desinstalar plugin de OpenCode"
  - "Perfiles SDD de OpenCode"
  - "Gestionar respaldos"
  - "Reiniciar almacén de revisiones"
  - "Revisión formal RDD"
  - "Desinstalación gestionada"
  - "Herramientas y plugins comunitarios"
  - "Salir"

#### Scenario: Microtextos de navegación y encabezados
- **DADO QUE** se visualiza el frame del menú
- **CUANDO** se renderizan los textos fijos
- **ENTONCES** la cabecera de la lista debe mostrar "Menú"
- **Y** la barra inferior de ayuda debe mostrar `j/k: navegar • enter: seleccionar • q: salir`.

---

### Requirement: Localización de Submenús y Pantallas de Configuración (REQ-16.3)
Las pantallas subordinadas de configuración y gestión deben desplegar sus títulos, opciones y botones de acción en castellano.

#### Scenario: Pantalla de Configuración de Modelos
- **DADO QUE** el usuario navega a "Configurar modelos"
- **CUANDO** se muestra la lista de opciones
- **ENTONCES** las opciones deben mostrarse en español (`Configurar modelos de Claude`, `Configurar modelos de OpenCode`, `Configurar modelos de Kiro`, `Configurar modelos de Codex`, y opción "Volver").

#### Scenario: Pantalla de Gestión de Respaldos
- **DADO QUE** el usuario navega a "Gestionar respaldos"
- **CUANDO** se presenta la lista y las acciones disponibles
- **ENTONCES** los botones y estados deben mostrarse en español (`Crear respaldo`, `Restaurar`, `Cancelar`, `Eliminar`, `Volver`, `Sin respaldos registrados`).

#### Scenario: Pantallas de Desinstalación y Herramientas Comunitarias
- **DADO QUE** el usuario accede a "Desinstalación gestionada" o "Herramientas y plugins comunitarios"
- **CUANDO** se despliegan los diálogos y confirmaciones
- **ENTONCES** los textos explicativos y advertencias deben formularse en español peninsular claro y profesional.

---

### Requirement: Determinismo en la Máquina de Estados y Navegación (REQ-16.4)
La traducción de los textos visibles NO debe alterar los índices numéricos de selección del cursor ni romper los contratos de navegación entre pantallas del modelo Bubbletea.

#### Scenario: Navegación por cursor intacta
- **DADO QUE** el cursor se sitúa en el índice 0 del menú principal
- **CUANDO** el usuario pulsa `enter`
- **ENTONCES** la acción disparada debe ser la transición al flujo de instalación (`ScreenDetection`), exactamente igual que en la versión en inglés.

#### Scenario: Validez de la suite de pruebas
- **DADO QUE** se ejecutan las pruebas unitarias de `internal/tui/screens/` y `internal/tui/`
- **CUANDO** se evalúan las aserciones sobre textos del menú
- **ENTONCES** todas las pruebas deben pasar al 100% reflejando el vocabulario oficial en español.

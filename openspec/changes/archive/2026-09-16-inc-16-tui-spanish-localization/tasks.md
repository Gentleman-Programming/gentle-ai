# Tareas de Implementación: Localización Integral de la TUI al Castellano (INC-16)

- [x] **1. Desacoplamiento de Advisories de Gentle AI**
  - [x] 1.1 Modificar `internal/update/advisory.go` para apuntar `advisoryURL` al repositorio de Axiom (`IGutierrezZ/axiom`).
  - [x] 1.2 Verificar que el inicio de la TUI sea limpio y no muestre líneas de advertencia ni enlaces ajenos.

- [x] **2. Localización al Castellano de la Pantalla de Bienvenida (`screens/welcome.go`)**
  - [x] 2.1 Traducir las opciones de `WelcomeOptions`: "Iniciar instalación", "Actualizar herramientas", "Sincronizar configuraciones", "Actualizar y sincronizar", "Configurar modelos", "Crear agente personalizado", "Plugins comunitarios de OpenCode", "Desinstalar plugin de OpenCode", "Perfiles SDD de OpenCode", "Gestionar respaldos", "Reiniciar almacén de revisiones", "Revisión formal RDD", "Desinstalación gestionada", "Herramientas y plugins comunitarios", "Salir".
  - [x] 2.2 Traducir microtextos de cabecera y navegación: "Menú", `welcomeHelpText` ("j/k: navegar • enter: seleccionar • q: salir"), `renderWelcomeMinimum` ("Iniciar instalación", "Ir").
  - [x] 2.3 Traducir prefijos de avisos ("Aviso: ", "Última versión: ", scroll "RePág/AvPág: desplazar").

- [x] **3. Localización de Submenús y Pantallas de Configuración**
  - [x] 3.1 Traducir `screens/model_config.go`: título, subtítulo, opciones ("Configurar modelos de Claude", "Volver", etc.) y barra de ayuda.
  - [x] 3.2 Traducir `screens/backups.go`: títulos, botones ("Restaurar", "Cancelar", "Eliminar", "Volver"), advertencias y confirmaciones.
  - [x] 3.3 Traducir `screens/detection.go`: título, opciones ("Continuar", "Volver"), etiquetas de estado del sistema.
  - [x] 3.4 Traducir `screens/preset.go`: título, etiquetas y descripciones de presets en castellano.
  - [x] 3.5 Traducir `screens/persona.go`: título, subtítulo y descripciones de personas.
  - [x] 3.6 Traducir `screens/common.go`: microtexto de espera ("Por favor, espera...").

- [x] **4. Adaptación de Pruebas Unitarias**
  - [x] 4.1 Actualizar `screens/welcome_test.go` para verificar las opciones traducidas y el ordenamiento.
  - [x] 4.2 Actualizar `screens/welcome_internal_test.go` para verificar el renderizado de la pantalla mínima y ayudas en español.
  - [x] 4.3 Actualizar `screens/model_config_test.go` y `screens/backups_test.go` con las nuevas aserciones en español.
  - [x] 4.4 Actualizar `internal/tui/model_test.go` en los tests que inspeccionan texto del menú principal.

- [x] **5. Verificación, Validación SDD y Archivo**
  - [x] 5.1 Ejecutar `go test ./internal/tui/...` y asegurar el 100% de éxito.
  - [x] 5.2 Compilar e instalar el binario global con `go install ./cmd/axiom`.
  - [x] 5.3 Validar formalmente el incremento con `axiom sdd verify-validate` y generar el reporte de verificación.
  - [x] 5.4 Archivar el incremento con `axiom sdd archive-compose` y actualizar `docs/ROADMAP.md`.

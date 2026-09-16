# Propuesta: Localización Integral en Castellano de la TUI Bubbletea y Desacoplamiento de Advisories (INC-16)

## Propósito (Intent)

Con la culminación de las Fases 1 y 2 de Axiom, el sistema cuenta con su propia identidad, persona de arquitectura en castellano peninsular (INC-10), comandos canónicos en la CLI (INC-13) y Dashboard Web (INC-15). Sin embargo, la interfaz gráfica interactiva de terminal (TUI Bubbletea) aún presentaba dos discrepancias importantes:

1. **Avisos de cabecera heredados de Gentle AI:** La pantalla de bienvenida seguía descargando y mostrando los avisos de estabilidad y la URL de release de Gentle AI v3.0.0 (`Latest release: ...` y `Advisory: Gentle AI v3.0.0...`), confundiendo al usuario y sugiriendo versiones de un proyecto externo.
2. **Textos y menús en inglés:** El menú de bienvenida (`WelcomeOptions`), los submenús de configuración (`ModelConfig`, `Backups`, `Profiles`, `CommunityTools`, `Detection`, `Preset`, `Persona`, `Upgrade`, `Sync`) y los atajos del pie de página (`j/k: navigate • enter: select • q: quit`) permanecen íntegramente en inglés, en contradicción con la **Regla Suprema de Idioma Obligatorio en Español (Castellano)** de Axiom.

El **Incremento 16 (INC-16: `inc-16-tui-spanish-localization`)** resuelve estas dos necesidades:
- Elimina cualquier vinculación remota de avisos hacia Gentle AI, apuntando al repositorio de Axiom (`IGutierrezZ/axiom`).
- Localiza de manera exhaustiva y profesional al castellano peninsular todos los textos de la TUI: menú principal, submenús, etiquetas de estado, diálogos de confirmación y barras de ayuda.

---

## Alcance (Scope)

### Dentro de Alcance (In Scope)

1. **Desacoplamiento de Avisos y Cabecera:**
   - Desvincular `advisoryURL` en `internal/update/advisory.go` para apuntar a `IGutierrezZ/axiom` (o fail-open sin avisos ajenos).
   - Eliminar las líneas de cabecera que referencian releases o comunicados de Gentle AI.

2. **Localización al Castellano del Menú Principal (`screens/welcome.go`):**
   - Traducir las 14-15 opciones de `WelcomeOptions`:
     * `Start installation` ➔ `Iniciar instalación`
     * `Upgrade tools` ➔ `Actualizar herramientas` (`Actualizar herramientas (al día)` / `Actualizar herramientas ★`)
     * `Sync configs` ➔ `Sincronizar configuraciones`
     * `Upgrade + Sync` ➔ `Actualizar y sincronizar`
     * `Configure models` ➔ `Configurar modelos`
     * `Create your own Agent` ➔ `Crear agente personalizado` (`sin motores`)
     * `OpenCode Community Plugins` ➔ `Plugins comunitarios de OpenCode`
     * `Uninstall OpenCode Plugin` ➔ `Desinstalar plugin de OpenCode`
     * `OpenCode SDD Profiles` ➔ `Perfiles SDD de OpenCode`
     * `Manage backups` ➔ `Gestionar respaldos`
     * `Reset review store` ➔ `Reiniciar almacén de revisiones`
     * `Receipt-Driven Development` ➔ `Revisión formal RDD`
     * `Managed uninstall` ➔ `Desinstalación gestionada`
     * `Community Tools/Plugins` ➔ `Herramientas y plugins comunitarios`
     * `Quit` ➔ `Salir`
   - Traducir títulos y pistas de navegación:
     * `Menu` ➔ `Menú`
     * `j/k: navigate • enter: select • q: quit` ➔ `j/k: navegar • enter: seleccionar • q: salir`
     * `PgUp/PgDn: scroll` ➔ `RePág/AvPág: desplazar`

3. **Localización de Pantallas y Submenús Clave:**
   - **Configuración de Modelos (`screens/model_config.go`):** Títulos, opciones por fase (`Explore`, `Propose`, `Spec`, `Design`, `Tasks`, `Apply`, `Verify`, `Archive`), selector y botón de volver.
   - **Gestión de Respaldos (`screens/backups.go`):** Lista de respaldos, acciones (`Crear respaldo`, `Restaurar`, `Fijar/Desfijar`, `Renombrar`, `Eliminar`).
   - **Detección y Presets (`screens/detection.go`, `screens/preset.go`, `screens/persona.go`):** Detección de agentes del sistema, presets de instalación y selección de persona (`Axiom`, `Gentleman`, `Neutral`).
   - **Operaciones de Actualización y Sincronización (`screens/upgrade.go`, `screens/sync_screen.go`, `screens/upgrade_sync.go`):** Indicadores de progreso, resultados y botones de continuación.
   - **Desinstalación y Plugins (`screens/uninstall.go`, `screens/opencode_plugin_uninstall.go`):** Diálogos de confirmación y desinstalación selectiva.

4. **Suite de Pruebas Unitarias:**
   - Adaptar las aserciones de pruebas en `internal/tui/` y `internal/tui/screens/` para validar el nuevo léxico en español y verificar que no se rompe ninguna máquina de estados de navegación (que opera por índices numéricos).

### Fuera de Alcance (Out of Scope)
- Modificaciones estructurales en el protocolo Bubbletea o en los adaptadores CLI subyacentes.

---

## Enfoque de Implementación (Approach)
1. **Fase spec:** Especificar formalmente los requerimientos de localización de menús y submenús con escenarios BDD en `spec.md`.
2. **Fase design:** Identificar el mapeo sistemático de microtextos en `screens/` y `model.go`.
3. **Fase tasks:** Desglosar la traducción por pantallas y suites de test.
4. **Fase apply:** Aplicar traducciones y actualizar aserciones.
5. **Fase verify & archive:** Validar la suite completa en verde y archivar el incremento.

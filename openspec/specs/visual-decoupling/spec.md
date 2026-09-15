# Especificación de Requerimientos: Desacoplamiento Visual y Limpieza de Marca (Axiom)

## Propósito

Definir de forma exhaustiva y verificable los requerimientos y escenarios de prueba para la desvinculación estética de Gentle AI, la supresión del logo en OpenCode, la no-intrusión en temas de usuario y la estandarización de nombres en adaptadores.

---

## 1. Capacidad: `opencode-logo-suppression`

Eliminación de la inyección de logos y plugins visuales no solicitados en la terminal de OpenCode.

### Requirement: Supresión de gentle-logo en presets de instalación (REQ-09.1)
El instalador y sincronizador de componentes NO DEBE incluir `ComponentOpenCodeGentleLogo` en la lista de componentes por defecto ni escribir `gentle-logo.tsx` en `~/.config/opencode/tui-plugins/`.

#### Scenario: Instalación estándar en OpenCode sin logo heredado
- **DADO** un entorno limpio donde OpenCode está seleccionado como agente
- **CUANDO** se ejecuta la instalación con el preset por defecto
- **ENTONCES** no se genera el archivo `~/.config/opencode/tui-plugins/gentle-logo.tsx`
- **Y** el archivo `~/.config/opencode/tui.json` no contiene ninguna entrada referenciando a `gentle-logo`

#### Scenario: Desinstalación o saneamiento de logo existente
- **DADO** un entorno donde previamente existía `~/.config/opencode/tui-plugins/gentle-logo.tsx`
- **CUANDO** se ejecuta el proceso de sincronización o desinstalación
- **ENTONCES** el archivo `gentle-logo.tsx` es eliminado del sistema de archivos
- **Y** la clave correspondiente es purgada de `tui.json`

---

## 2. Capacidad: `theme-non-intrusiveness`

Respeto de las preferencias estéticas del desarrollador sin forzar esquemas de colores en los editores.

### Requirement: No forzado de temas en settings.json (REQ-09.2)
El componente de temas NO DEBE modificar las claves `"theme"` en los archivos `settings.json` de Claude Code ni de OpenCode salvo petición explícita y consciente del usuario.

#### Scenario: Preservación del tema existente del usuario
- **DADO** un archivo `settings.json` con `"theme": "kanagawa"` o `"theme": "nord"`
- **CUANDO** se ejecuta la sincronización de Axiom
- **ENTONCES** el valor de `"theme"` permanece inalterado
- **Y** no se sobreescribe con `"gentleman"` ni ninguna otra paleta fija

### Requirement: Renombrado de archivos de temas opcionales (REQ-09.3)
Los activos de tema instalables voluntariamente DEBEN nombrarse `axiom.json` y `axiom-dark.json` en lugar de `gentleman.json` y `gentleman-cute.json`.

#### Scenario: Generación de temas opcionales Axiom
- **DADO** un usuario que selecciona la instalación voluntaria de temas visuales
- **CUANDO** se despliegan los temas en `~/.claude/themes/` o `~/.config/opencode/themes/`
- **ENTONCES** los archivos creados son `axiom.json` y `axiom-dark.json`
- **Y** no se crea ningún archivo con el nombre `gentleman.json`

---

## 3. Capacidad: `adapter-brand-alignment`

Alineación de nombres de archivos de configuración e instrucciones hacia el estándar de Axiom.

### Requirement: Rutas de instrucciones con nomenclatura Axiom (REQ-09.4)
Los adaptadores de agentes DEBEN utilizar rutas y nombres de archivos que contengan el prefijo o identificador `axiom` en lugar de `gentle-ai`.

#### Scenario: Archivo de instrucciones en VS Code Copilot
- **DADO** el adaptador de VS Code Copilot
- **CUANDO** se consulta la ruta del archivo de system prompt mediante `SystemPromptFile()`
- **ENTONCES** el nombre del archivo retornado es `axiom.instructions.md` (ubicado en la carpeta de prompts del usuario)

#### Scenario: Archivo de steering en Kiro IDE
- **DADO** el adaptador de Kiro IDE
- **CUANDO** se genera el archivo de steering
- **ENTONCES** la ruta objetivo es `~/.kiro/steering/axiom.md`

#### Scenario: Directorio de modelos en Pi
- **DADO** el adaptador de Pi
- **CUANDO** se resuelve la configuración de modelos
- **ENTONCES** la ruta de configuración apunta a `~/.pi/axiom/`

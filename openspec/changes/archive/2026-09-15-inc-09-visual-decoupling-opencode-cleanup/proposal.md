# Propuesta: Desacoplamiento Visual, Eliminación del Logo Gentle AI en OpenCode y Neutralización de Temas (INC-09)

## Propósito (Intent)

Axiom nace como una plataforma de desarrollo y orquestación autónoma e independiente. Sin embargo, al haberse bifurcado de Gentle-AI, arrastra componentes de inyección visual fuertemente ligados a la identidad de la herramienta madre:
1. En **OpenCode**, el plugin `gentle-logo.tsx` inyecta automáticamente en el slot `home_logo` un arte en caracteres Braille/ASCII con la silueta de una rosa y la inscripción `✦ Gentle AI ✦`.
2. En **Claude Code** y **OpenCode**, el sistema instala archivos de tema denominados `gentleman.json` y `gentleman-cute.json`, e inyecta forzosamente `"theme": "gentleman"` en la configuración (`settings.json`) de los editores, alterando la paleta de colores del usuario sin su consentimiento.
3. En **VS Code Copilot**, **Kiro IDE** y **Pi**, los adaptadores escriben instrucciones del sistema en archivos con nombres heredados (`gentle-ai.instructions.md`, `~/.kiro/steering/gentle-ai.md`, `~/.pi/gentle-ai/`).
4. El repositorio raíz contiene banners y logos antiguos de Gentle AI en `docs/assets/brand/` y en `README.md`.

El objetivo de este incremento es **purgar toda imposición estética no deseada**, respetar la configuración del usuario sin forzar temas y renombrar todos los assets y rutas hacia la identidad limpia de **Axiom**.

---

## Alcance (Scope)

### Dentro de Alcance (In Scope)

1. **Eliminación del plugin de logo Gentle AI en OpenCode:**
   - Retirar `ComponentOpenCodeGentleLogo` (`opencode-gentle-logo`) de los presets de instalación (`PresetFullGentleman` / futuros presets de Axiom).
   - Eliminar o neutralizar la generación de `~/.config/opencode/tui-plugins/gentle-logo.tsx` en `internal/components/opencodeplugin/plugin.go`.
   - Garantizar que el slot `home_logo` de OpenCode quede limpio o respete la configuración por defecto de OpenCode.
   - El desinstalador debe retirar `gentle-logo.tsx` si ya existía en la máquina del usuario.

2. **Neutralización y Renombrado de Temas Visuales:**
   - Eliminar la inyección obligatoria de `"theme": "gentleman"` en los `settings.json` de OpenCode y Claude Code (`internal/components/theme/inject.go`). Axiom no forzará ningún tema por defecto sobre el entorno del usuario.
   - Renombrar los activos de tema disponibles a `axiom.json` y `axiom-dark.json` (manteniendo una paleta neutra y profesional si el usuario decide seleccionarlos voluntariamente).

3. **Renombrado de Rutas de Instrucciones en Adaptadores:**
   - En **VS Code Copilot** (`internal/agents/vscode/adapter.go`): cambiar `gentle-ai.instructions.md` a `axiom.instructions.md`.
   - En **Kiro IDE** (`internal/agents/kiro/`): cambiar `~/.kiro/steering/gentle-ai.md` a `~/.kiro/steering/axiom.md`.
   - En **Pi** (`internal/agents/pi/`): cambiar `~/.pi/gentle-ai/` a `~/.pi/axiom/`.

4. **Identidad Visual del Repositorio:**
   - Actualizar `README.md` para reflejar la identidad de Axiom como plataforma SDD Multi-Rol.
   - Retirar banners y logos de la rosa de Gentle AI de `docs/assets/brand/` y sustituirlos por un banner o identificador sobrio y limpio de Axiom.

### Fuera de Alcance (Out of Scope)

- Modificación de la lógica del orquestador ni de los comandos de agentes (tratado en INC-11).
- Modificación del modelo de personas ni del dialecto voseo (tratado en INC-10).
- Modificación de los comandos CLI de `axiom` ni integración de `sdd-status` (tratado en INC-13).

---

## Plan de Pruebas y Validación

- Pruebas unitarias en `internal/components/opencodeplugin` verificando que no se instala `gentle-logo.tsx`.
- Pruebas unitarias en `internal/components/theme` verificando que no se altera `settings.json` con `"theme": "gentleman"`.
- Pruebas de adaptadores de VS Code, Kiro y Pi verificando las nuevas rutas de archivos de instrucciones `axiom.*`.

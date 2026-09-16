# Propuesta: Unificación de TUI Bubbletea, Comandos de Ecosistema en CLI axiom y Pasarela de gentle-ai (INC-14)

## Propósito (Intent)

Con los incrementos INC-01 a INC-13 se consolidó el núcleo arquitectónico de Axiom (topologías de repositorios, handoffs estructurados, concurrencia multi-rol, dashboard web local, autoskills supervisadas, grafo de código semántico, documentación viva, hub multi-proyecto, orquestador de agentes, variables de entorno `AXIOM_*` y comandos SDD/RDD nativos).

No obstante, subsiste una escisión evidente entre dos binarios:
1. **El binario `axiom`:** Gobierna la gestión del workspace, el hub y los comandos SDD, pero al ser ejecutado sin argumentos solo muestra texto de ayuda plano y carece de los comandos interactivos de instalación y sincronización de herramientas de agentes.
2. **El binario `gentle-ai`:** Mantiene cautivos los comandos de instalación (`install`), sincronización (`sync`), actualización (`upgrade`), diagnóstico (`doctor`), respaldos (`backup`/`restore`) y la interfaz gráfica de terminal (TUI) de Bubbletea (`internal/tui`).
3. **Identidad visual heredada:** La TUI de Bubbletea sigue mostrando el arte ASCII de la rosa de Gentle AI y el lema publicitario original en su pantalla de bienvenida (`internal/tui/styles/logo.go` y `styles.go`).

El **Incremento 14 (INC-14: `axiom-tui-branding-and-cli-cutover`)** culmina la transición definitiva:
- Unifica todos los comandos de instalación, sincronización, actualización y diagnóstico de agentes bajo el binario canónico **`axiom`**.
- Incorpora el lanzamiento de la TUI interactiva de Bubbletea en `axiom` (por defecto al invocar sin argumentos en terminal interactivo TTY, o explícitamente con `axiom tui`).
- Renueva la identidad gráfica de la TUI con un logotipo ASCII moderno y sobrio de **AXIOM** y el lema oficial de la plataforma.
- Convierte el binario secundario `gentle-ai` en un envoltorio (*wrapper*) de compatibilidad que redirige a `axiom` con un aviso informativo de deprecación.

---

## Alcance (Scope)

### Dentro de Alcance (In Scope)

1. **Renovación de Marca e Identidad en la TUI (`internal/tui/`):**
   - Sustituir el arte ASCII de la rosa de Gentle AI en `internal/tui/styles/logo.go` por un logotipo tipográfico limpio y profesional de **AXIOM** en ASCII art con gradientes Lipgloss.
   - Actualizar el lema de bienvenida en `internal/tui/styles/styles.go` a `"Axiom " + version + " — Plataforma SDD Multi-Rol y Multi-Repositorio"`.
   - Adaptar microtextos y títulos en las pantallas de la TUI (`internal/tui/screens/welcome.go`) a la identidad de Axiom.

2. **Integración de TUI y Comandos de Ecosistema en `cmd/axiom/main.go`:**
   - Ejecución interactiva por defecto: Si `len(os.Args) < 2` y la sesión cuenta con TTY de entrada y salida, lanzar la TUI interactiva de Bubbletea; en caso contrario, emitir `printHelp()`.
   - Subcomando explícito `axiom tui` para lanzar la interfaz de terminal bajo demanda.
   - Integración nativa de subcomandos de gestión de agentes y herramientas:
     - `axiom install [flags]`
     - `axiom sync [flags]`
     - `axiom upgrade [flags]`
     - `axiom doctor [flags]`
     - `axiom backup [list|restore|delete|rename]`
     - `axiom restore [flags]`
     - `axiom uninstall [flags]`
   - Actualización completa de la ayuda en `printHelp()`.

3. **Pasarela de Compatibilidad en `cmd/gentle-ai/main.go`:**
   - Transformar el punto de entrada de `gentle-ai` en un wrapper transparente.
   - Emitir en `stderr` el aviso: `Aviso: 'gentle-ai' está deprecado y ha sido unificado en 'axiom'. Ejecutando 'axiom [args]'...`.
   - Delegar la ejecución conservando todos los argumentos y códigos de salida.

4. **Batería de Pruebas Unitarias y de Integración:**
   - Pruebas del render del logotipo y lema de Axiom en `internal/tui/styles/`.
   - Pruebas en `cmd/axiom/main_test.go` validando el despacho de `install`, `sync`, `upgrade`, `tui` y la detección de TTY.
   - Pruebas en `cmd/gentle-ai/main_test.go` comprobando el aviso de redirección y la preservación de comportamiento.

### Fuera de Alcance (Out of Scope)

- Modificación de la lógica interna de los pipelines de instalación de agentes de terceros (`internal/pipeline/`).
- Alteración de los formatos de especificaciones OpenSpec ni de la base de datos de revisión RDD.

---

## Plan de Pruebas y Validación

- `go test -v ./internal/tui/...` para asegurar que el render de estilos y pantallas no sufra regresiones.
- `go test -v ./cmd/axiom/...` para verificar el enrutamiento y la ayuda actualizada.
- `go test -v ./cmd/gentle-ai/...` para validar la pasarela de compatibilidad.
- Compilación de ambos binarios (`axiom` y `gentle-ai`) y verificación en vivo.
- Validación formal con `axiom sdd verify-validate` y sincronización con `axiom archive sync`.

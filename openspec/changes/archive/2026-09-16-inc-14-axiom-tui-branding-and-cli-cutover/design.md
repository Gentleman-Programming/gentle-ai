# Diseño de Arquitectura: Unificación de TUI Bubbletea, Comandos de Ecosistema en CLI axiom y Pasarela de gentle-ai (INC-14)

## 1. Visión General y Diagrama de Flujo

El objetivo arquitectónico es convertir el binario `axiom` en el punto de entrada integral y unificado de toda la plataforma, integrando la interfaz TUI de Bubbletea y los comandos de aprovisionamiento de herramientas, mientras se mantiene `gentle-ai` como una pasarela ligera de compatibilidad retroactiva.

```
Usuario / Terminal
      │
      ├── Invoca: axiom [sin args en TTY]  ────┐
      ├── Invoca: axiom tui ───────────────────┤
      │                                        ▼
      │                             ┌───────────────────────┐
      │                             │  TUI Bubbletea Axiom  │
      │                             │   (Logo AXIOM ASCII   │
      │                             │  Lema Plataforma SDD) │
      │                             └───────────────────────┘
      │
      ├── Invoca: axiom <install|sync|upgrade|doctor|backup|restore|uninstall>
      │         │
      │         ▼
      │   ┌───────────────────────────────────────────────┐
      │   │  internal/app.RunArgs (Motor de Ecosistema)   │
      │   └───────────────────────────────────────────────┘
      │
      └── Invoca: gentle-ai [args]
                │
                ▼
          ┌───────────────────────────────────────────────┐
          │  cmd/gentle-ai/main.go (Pasarela Deprecada)   │
          │   1. Emite advertencia en stderr              │
          │   2. Delega en internal/app.RunArgs           │
          └───────────────────────────────────────────────┘
```

---

## 2. Decisiones de Diseño

### D-1: Detección de Terminal Interactivo (TTY) en `cmd/axiom/main.go`
- **Decisión:** Usar `mattn/go-isatty` (ya presente en las dependencias indirectas del proyecto) o la función canónica `isatty.IsTerminal` para evaluar `os.Stdin.Fd()` y `os.Stdout.Fd()`.
- **Comportamiento:**
  - Si ambos descriptores son terminales interactivos: al invocar `axiom` sin parámetros se abre directamente la TUI interactiva (`app.RunArgs([]string{}, os.Stdout)`).
  - Si alguno de los dos descriptores no es un terminal (por ejemplo en pipes, redirecciones o CI sin TTY): se imprime el texto de ayuda `printHelp()` con código de salida `0`, evitando bloqueos o errores de renderizado en headless.

### D-2: Identidad Tipográfica de Axiom en `internal/tui/styles/logo.go`
- **Decisión:** Reemplazar el arte ASCII de la flor de rosa (65 líneas con caracteres Braille/Unicode densos) por un diseño tipográfico horizontal de bloque con las letras **AXIOM** de 6 a 8 líneas de altura.
- **Ventajas:**
  - Mantiene legibilidad impecable en terminales de anchura estándar (80 columnas).
  - Distribución uniforme del gradiente de 5 bandas de color Lipgloss (Mauve → Lavender → Blue → Teal → Green).
  - Eliminación absoluta de cualquier vestigio visual de Gentle AI en la experiencia de inicio.

### D-3: Reenvío y Advertencia en `cmd/gentle-ai/main.go`
- **Decisión:** La pasarela no ejecuta un subproceso hijo (evitando sobrecarga de memoria o problemas de señalización en Windows), sino que importa y llama directamente a `internal/app.RunArgs(os.Args[1:], os.Stdout)`, imprimiendo previamente un mensaje claro en `os.Stderr`:
  `fmt.Fprintln(os.Stderr, "Aviso: 'gentle-ai' ha sido unificado en 'axiom'. Se recomienda utilizar 'axiom' en su lugar.")`
- **Resultado:** Rendimiento idéntico, cero coste de proceso y máxima compatibilidad para scripts antiguos.

---

## 3. Desglose de Componentes Afectados

1. **`cmd/axiom/main.go`:**
   - Importar `github.com/gentleman-programming/gentle-ai/v2/internal/app`.
   - Ampliar el switch principal con: `tui`, `install`, `sync`, `upgrade`, `doctor`, `backup`, `restore`, `uninstall`.
   - Ajustar el caso `len(os.Args) < 2` con detección de TTY.
   - Actualizar `printHelp()`.
2. **`cmd/axiom/main_test.go`:**
   - Añadir casos de prueba para el enrutamiento de comandos de ecosistema y validación de ayuda.
3. **`cmd/gentle-ai/main.go`:**
   - Insertar la emisión de aviso de deprecación en `os.Stderr`.
4. **`internal/tui/styles/logo.go`:**
   - Rediseñar `logoLines` con la tipografía ASCII de AXIOM.
5. **`internal/tui/styles/styles.go`:**
   - Modificar `Tagline(version)`.
6. **`internal/tui/screens/welcome.go`:**
   - Asegurar que los textos del menú y encabezados sean coherentes con Axiom.

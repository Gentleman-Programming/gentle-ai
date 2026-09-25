# ODD: Migración Canónica del Módulo Go a Axiom y Soporte de Release

> **Documento vivo ODD.** Fichero autoritativo: `odd/tasks/migracion-modulo-go-canonico.md`.
> Espejo de recuperación en Engram: topic `odd/migracion-modulo-go-canonico/tasks`, proyecto `axiom`.

## Objetivo

Renombrar el módulo raíz de Go en `go.mod` a la ruta canónica del repositorio de Axiom (`github.com/IGutierrezZ/axiom/v3` o la ruta canónica acordada), refactorizar de forma atómica todas las rutas de importación internas en la base de código (`cmd/`, `internal/`), actualizar los manifiestos de empaquetado y release (`.goreleaser.yaml`, scripts de instalación, Dockerfiles) y garantizar la compilación limpia y ejecución de pruebas sin residuos del módulo upstream.

## Problema

1. El archivo `go.mod` aún declara `module github.com/gentleman-programming/gentle-ai/v3`.
2. Más de 600 ficheros Go en `internal/` y `cmd/` contienen rutas de importación absolutas dependientes de dicho módulo.
3. Los scripts de instalación automática vía Go toolchain (`go install ...`), scripts en PowerShell/Bash (`scripts/install.ps1`, `scripts/install.sh`) y definiciones de Goreleaser referencian el repositorio upstream `Gentleman-Programming/gentle-ai`.
4. Este cambio, aunque conceptualmente sencillo, es masivo en volumen (~10.000 líneas modificadas en import statements). Si se mezcla con cambios de lógica de negocio o de contratos, hace inviable la revisión, depuración y bisección de errores en Git. Debe planificarse y ejecutarse en una sesión y commit exclusivos.

## Alcance Autorizado

- `go.mod` y `go.sum`: Actualización de la directiva `module` y regeneración limpia con `go mod tidy`.
- Sustitución atómica de imports: Reemplazar el prefijo `github.com/gentleman-programming/gentle-ai/v3` por el nuevo prefijo canónico en todos los ficheros `.go` del repositorio.
- Scripts de build e instalación:
  - `scripts/install.sh`
  - `scripts/install.ps1`
  - `Dockerfile` / `Containerfile`
  - `.goreleaser.yaml` y `.goreleaser-provider-contract/`
- Documentación de instalación por Go: actualizar ejemplos de `go install ...` en guías de plataformas y README.
- Verificación exhaustiva de compilación (`go build ./cmd/...`), análisis estático (`go vet ./...`) y ejecución de pruebas unitarias.

## Restricciones

- **Cero cambios de lógica funcional:** Esta tarea es estrictamente mecánica. Ningún algoritmo, contrato ni estructura de datos debe modificarse durante el reemplazo de imports.
- **Atomicidad absoluta:** Todo el árbol debe compilar inmediatamente tras el cambio; no se admiten estados intermedios donde parte del código use el módulo antiguo y parte el nuevo.
- **Confirmación previa de URL canónica:** El nuevo nombre del módulo debe reflejar exactamente la ubicación del repositorio remoto para que `go install` resuelva correctamente en red.

---

## Tareas

- [ ] **T1 · Fijar la ruta canónica del módulo y preparar el script de sustitución**
  - Acordar con el usuario el module path canónico definitivo (ejemplo: `github.com/IGutierrezZ/axiom/v3`).
  - Preparar script de reemplazo recursivo de imports seguro y determinista para ficheros `.go`.

- [ ] **T2 · Reemplazo masivo de imports y actualización de `go.mod`**
  - Modificar la directiva `module` en `go.mod`.
  - Ejecutar la sustitución del prefijo en todos los ficheros de `cmd/`, `internal/` y suites de prueba.
  - Ejecutar `go mod tidy` para recalcular el árbol de dependencias y checksums en `go.sum`.

- [ ] **T3 · Actualización de scripts de instalación y distribución**
  - Actualizar `scripts/install.sh` y `scripts/install.ps1` para descargar y compilar desde el nuevo módulo.
  - Actualizar directivas de empaquetado en `.goreleaser.yaml` y Dockerfiles asociados.
  - Actualizar referencias en documentación a `go install <nuevo-modulo>/cmd/axiom@latest`.

- [ ] **T4 · Verificación de compilación, análisis estático y pruebas**
  - Ejecutar `go build ./cmd/axiom` y `go build ./cmd/gentle-ai`.
  - Ejecutar `go vet ./...` para asegurar que no existan inconsistencias estáticas.
  - Ejecutar la batería de tests unitarios del proyecto para confirmar que todos los paquetes pasan en verde.

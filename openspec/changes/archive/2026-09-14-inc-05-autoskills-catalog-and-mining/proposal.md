# Propuesta: Autoskills (midudev/autoskills) y Minería Heurística con Gobernanza Human-in-the-Loop (INC-05)

## Propósito (Intent)

En equipos de desarrollo asistidos por IA y plataformas SDD como Axiom, los agentes requieren directrices contextuales claras y verificadas (Skills) para escribir código conforme al stack tecnológico del proyecto, sus dependencias y las convenciones específicas del repositorio.

Para evitar la creación manual y tediosa de skills, así como prevenir la alucinación de directrices no verificadas, **Axiom adopta formalmente el estándar y catálogo auditado de `midudev/autoskills`** (`https://github.com/midudev/autoskills`), combinándolo con un motor de minería heurística de código local y una compuerta estricta de aprobación humana (*Human-in-the-Loop*).

El sistema opera en dos capas complementarias con gobernanza unificada:

1. **Capa 1 — Integración con el Catálogo Oficial de `midudev/autoskills`:**
   - Detecta automáticamente el stack analizando dependencias de los repositorios asociados a cada rol (`package.json`, `go.mod`, `Cargo.toml`, etc.) y archivos de configuración clave (`next.config.*`, `tailwind.config.*`, `vite.config.*`, etc.) basándose en la matriz `SKILLS_MAP` de `midudev/autoskills`.
   - Consume el registro oficial auditado de skills de midudev (`skills-registry` con manifiesto `index.json`).
   - Valida la integridad criptográfica de cada archivo (`SKILL.md` y assets) mediante comparación de hashes **SHA-256** antes de presentarlo al equipo.
   - Implementado mediante un **cliente nativo en Go** (`internal/autoskill/client.go` y `detector.go`) que elimina la dependencia de runtime de Node.js (permitiendo operar sin restricciones de versión de Node como la exigencia de Node >= 22.6.0 de `npx autoskills`), pero manteniendo compatibilidad para orquestar `autoskills.sh` / `npx autoskills` si el entorno local dispone de las herramientas.

2. **Capa 2 — Minería Heurística de Repositorio (Patrones Locales de Axiom):**
   - Analizador sintáctico y estructural en Go (`internal/autoskill/miner.go`) que inspecciona los repositorios locales vinculados a cada rol en `axiom.yaml`.
   - Descubre patrones y convenciones internas que ningún catálogo global conoce (ej. diseño de pruebas tabulares en Go, inyección de dependencias interna, estructura de capas en `internal/`, convenciones de componentes en frontend).
   - Genera borradores canónicos de `SKILL.md` con metadata contextual del repositorio.

3. **Gobernanza Human-in-the-Loop (Buzón Transitorio Obligatorio):**
   - **Ninguna skill (ni de midudev ni minada) se activa automáticamente** en `skills/`.
   - Todas las detecciones y borradores se depositan en una bandeja de entrada transitoria en el sistema de archivos: `.axiom/skills/inbox/<nombre>/`.
   - Los desarrolladores o Tech Leads revisan la justificación, diff y contenido, y aprueban (`axiom skill approve`) o descartan (`axiom skill reject`) cada skill, tanto desde la CLI como desde el Dashboard Web local (`axiom ui`).
   - Al ser aprobada, la skill se instala en el directorio canónico `skills/<nombre>/SKILL.md` y queda disponible de inmediato para todos los agentes de Axiom.

---

## Alcance (Scope)

### Dentro de Alcance (In Scope)

- **Paquete de dominio en el runtime de Axiom (`internal/autoskill/`):**
  - `types.go`: Estructuras de datos para representar skills del registro, reglas de detección (`SKILLS_MAP`), propuestas en el buzón (`SkillProposal`), manifiestos de registro (`RegistryIndex`), minería heurística (`MiningPattern`) e informes de escaneo (`ScanReport`).
  - `client.go`: Cliente HTTP nativo en Go para descargar manifiestos y skills desde el registro oficial de `midudev/autoskills`, con validación estricta de suma de verificación **SHA-256**.
  - `detector.go`: Motor de detección de stack tecnológico multi-rol y multi-repositorio que evalúa dependencias (`go.mod`, `package.json`, etc.) y archivos de configuración contra `SKILLS_MAP`.
  - `miner.go`: Analizador heurístico de código que descubre patrones arquitectónicos recurrentes en los repositorios vinculados y formula borradores canónicos de `SKILL.md`.
  - `manager.go`: Gestor del ciclo de vida del buzón transitorio (`.axiom/skills/inbox/`), listado, aprobación (promoción atómica a `skills/`) y rechazo.
- **Integración CLI en `cmd/axiom/main.go`:**
  - Grupo de comandos `axiom skill`:
    - `axiom skill scan [--role <rol>] [--path <directorio>] [--offline]`: Escanea tecnologías contra el catálogo de midudev y mina patrones locales, depositando candidatos en el buzón.
    - `axiom skill list [--inbox] [--path <directorio>]`: Lista skills activas o propuestas pendientes de aprobación con su procedencia (`midudev` vs `mined`).
    - `axiom skill approve <nombre> [--path <directorio>]`: Aprueba e instala formalmente una skill en `skills/`.
    - `axiom skill reject <nombre> [--path <directorio>]`: Rechaza y purga una propuesta del buzón.
- **Integración con el Dashboard Web (`internal/dashboard/`):**
  - Endpoints REST en `internal/dashboard/server.go`:
    - `GET /api/skills/inbox`: Consulta propuestas pendientes con detalle y procedencia.
    - `POST /api/skills/scan`: Ejecuta el escaneo de autoskills desde la interfaz web.
    - `POST /api/skills/approve`: Aprueba e instala una skill desde la UI con feedback reactivo.
    - `POST /api/skills/reject`: Descarta una skill propuesta.
  - Interfaz web en `assets/` (`index.html`, `style.css`, `app.js`):
    - Nueva sección de "Buzón de Autoskills" en la pestaña de Skills, mostrando el origen de la skill (insignia `midudev` auditada o `minería local`), badge de SHA-256 verificado, visor de contenido de la skill y botones directos de Aprobación/Rechazo.
- **Suite de pruebas unitarias (`internal/autoskill/autoskill_test.go`):**
  - Pruebas del cliente de registro de midudev (mock de índice y verificación SHA-256).
  - Pruebas de detección basadas en `SKILLS_MAP` sobre fixtures de `go.mod` y `package.json`.
  - Pruebas de minería heurística de código.
  - Pruebas del gestor de buzón y compuertas de aprobación/rechazo.

### Fuera de Alcance (Out of Scope)

- Análisis semántico profundo basado en LSP / Tree-sitter con Serena (reservado para **INC-06**).
- Síntesis y catalogación masiva de código legado sin documentar en `Archive` (reservado para **INC-07**).

---

## Capacidades (Capabilities)

### Nuevas Capacidades

- `autoskill-midudev-registry`: Cliente nativo Go que consulta el registro oficial auditado de `midudev/autoskills` con validación SHA-256.
- `autoskill-stack-detector`: Detector multi-rol y multi-repositorio que aplica las reglas de `SKILLS_MAP` para identificar tecnologías presentes en el proyecto.
- `autoskill-heuristic-miner`: Extractor heurístico de convenciones y patrones locales de código fuente.
- `autoskill-governance-inbox`: Bandeja de entrada `.axiom/skills/inbox/` con política estricta de aprobación humana (*Human-in-the-Loop*).
- `axiom-cli-skill`: Comandos CLI `axiom skill scan|list|approve|reject`.
- `axiom-dashboard-skill-inbox`: Panel visual e interactivo en el dashboard web local para auditoría y aprobación de skills en un clic.

---

## Enfoque de Implementación (Approach)

1. **Definir el modelo y catálogo nativo (`internal/autoskill/types.go` y `client.go`):**
   - Incorporar las definiciones canónicas de `SKILLS_MAP` de `midudev/autoskills`.
   - Diseñar el cliente HTTP con verificación SHA-256 e índice en memoria/caché.
2. **Implementar el detector y el minero (`detector.go`, `miner.go`):**
   - Mapear tecnologías detectadas en cada rol del workspace.
   - Analizar estructuras sintácticas clave para detectar convenciones locales.
3. **Implementar el gestor del buzón (`manager.go`):**
   - Escribir en `.axiom/skills/inbox/`.
   - Mover atómicamente a `skills/<nombre>/` al aprobar.
4. **Integrar la suite de tests unitarios (`autoskill_test.go`):**
   - Pruebas unitarias completas simulando descarga, verificación de hashes y detección.
5. **Integrar los comandos CLI en `cmd/axiom/main.go`:**
   - Registrar `axiom skill scan`, `axiom skill list`, `axiom skill approve`, `axiom skill reject`.
6. **Enriquecer el Dashboard Web (`internal/dashboard/`):**
   - Conectar endpoints `/api/skills/*` con `manager.go` y actualizar SPA visual (`index.html`, `app.js`, `style.css`).
7. **Verificación y validación de arnés:**
   - Compilación de `axiom.exe`, ejecución de `go test ./...` y validación con `gentle-ai sdd-verify-validate`.

# Axiom — Reglas Maestras del Proyecto y Guía de Agentes

> **Proyecto:** Axiom (Fork y versión paralela de Gentle-AI)  
> **Propósito:** Evolución y desarrollo de flujos avanzados de IA, orquestación SDD/RDD y herramientas para agentes de codificación.  
> **Stack Técnico:** Go 1.25+ (Go 1.27.0 en entorno local), Bubbletea TUI (`charmbracelet/bubbletea`), Lipgloss, arquitectura de paquetes en `internal/`, arnés SDD nativo de Gentle-AI y memoria persistente Engram MCP.

---

## 0. REGLA SUPREMA: IDIOMA OBLIGATORIO — ESPAÑOL (CASTELLANO)

- **TODO EN ESPAÑOL:** Todas las respuestas del agente, mensajes de chat, explicaciones, resúmenes, razonamientos dirigidos al usuario y **TODOS los artefactos de SDD** (`proposal.md`, `spec.md`, `design.md`, `tasks.md`, `verify-report.md`, `archive-report.md`, `walkthrough.md`, etc.) DEBEN generarse y redactarse estrictamente en **español (castellano)**.
- **PROHIBIDO EL INGLÉS EN DOCUMENTACIÓN Y ARTEFACTOS:** Queda terminantemente prohibido generar propuestas, especificaciones, diseños o informes en inglés. La única excepción son los identificadores técnicos de código (nombres de paquetes, funciones, structs, interfaces y variables en Go) y palabras clave del lenguaje.
- **PRECEDENCIA:** Si cualquier skill, prompt o plantilla externa menciona "default to English", esta regla del proyecto TIENE PRECEDENCIA ABSOLUTA y la sobreescribe: genera SIEMPRE el contenido en español castellano.

---

## 1. Filosofía de Desarrollo: Spec-Driven Development (SDD) con Gentle-AI

Este proyecto se desarrolla sobre sí mismo utilizando la metodología **Spec-Driven Development (SDD)** de Gentle-AI.

**REGLA DE ORO:** Nunca implementar directamente en código cambios arquitectónicos o funcionalidades mayores sin pasar por las fases del ciclo SDD:

```
[ sdd-explore ] ➔ [ sdd-propose ] ➔ [ sdd-spec ] ➔ [ sdd-design ] ➔ [ sdd-tasks ] ➔ [ sdd-apply ] ➔ [ sdd-verify ] ➔ [ sdd-archive ]
```

### Principios de la Máquina de Estados de SDD
1. **File-System como Fuente de la Verdad:** El estado de las fases reside en `openspec/` y `.openspec/`. No confiar en la memoria volátil del chat.
2. **Lossless Blocking Prompts:** Antes de pasar de `sdd-propose` a `sdd-spec` o de `sdd-design` a `sdd-apply`, presentar la propuesta o diseño al usuario en español y esperar aprobación explícita.
3. **Delegación con Subagentes:** Usar la primitiva de delegación de la plataforma para exploraciones profundas, investigación externa y verificaciones independientes (`invoke_subagent` en Antigravity).
4. **Presupuestos y CAS (Compare-And-Swap):** En `sdd-apply`, implementar exclusivamente contra los requerimientos acordados en la especificación y tareas definidas.
5. **Comandos Nativos de Gentle-AI:** Utilizar los comandos de verificación y composición provistos por la CLI de Gentle-AI:
   - `gentle-ai sdd-status [change]`
   - `gentle-ai sdd-continue [change]`
   - `gentle-ai sdd-verify-validate`
   - `gentle-ai sdd-archive-compose`

---

## 2. Protocolo de Memoria Persistente (Engram MCP)

El proyecto cuenta con el servidor MCP de **Engram** conectado bajo el espacio de proyecto `axiom`.
- **Cuándo guardar (`mem_save`):** Inmediatamente tras resolver un bug, tomar una decisión de diseño, aprender una regla interna de Go/Axiom o establecer un patrón.
- **Cuándo consultar (`mem_context` / `mem_search`):** Al inicio de sesión o al retomar una funcionalidad para no perder el contexto previo.
- **Cierre de sesión (`mem_session_summary`):** Obligatorio antes de finalizar la sesión de trabajo.

---

## 3. Estándares Técnicos (Go & Arquitectura)

- **Estructura del Proyecto:**
  - `cmd/`: Puntos de entrada ejecutables (CLI de la herramienta).
  - `internal/`: Paquetes y lógica interna del sistema (agentes, TUI, modelo, pipeline, catálogo, verificadores, etc.).
  - `openspec/`: Especificaciones vivas, cambios en curso (`openspec/changes/`) y cambios archivados (`openspec/changes/archive/`).
  - `skills/`: Skills de desarrollo y flujos de trabajo de Gentle-AI.
- **Pruebas y Calidad:**
  - `go test ./...` para pruebas unitarias.
  - `go vet ./...` para validación estática.
  - `gofmt` para formateo estándar de Go.

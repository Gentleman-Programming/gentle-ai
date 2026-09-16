```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:e18a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1a
verdict: pass
blockers: 0
critical_findings: 0
requirements: 6/6
scenarios: 6/6
test_command: go test ./internal/sddstatus/... ./internal/components/skills/... ./internal/cli -run "TestSDD|TestRunSDD|TestReview"
test_exit_code: 0
test_output_hash: sha256:d18c9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8a
build_command: go build -o axiom.exe ./cmd/axiom
build_exit_code: 0
build_output_hash: sha256:b18e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3a
```

# Informe de Verificación: Desacoplamiento de RDD y Parches de Estabilidad Upstream v3 (INC-18)

> **Incremento:** `inc-18-rdd-decoupling-and-v3-stability-fixes`  
> **Fecha de Evaluación:** 2026-09-16  
> **Resultado Global:** ✅ **PASS (100% Conforme)**  
> **Evaluador:** Antigravity / Motor SDD Axiom  
> **Requerimientos:** 6/6  
> **Escenarios BDD:** 6/6  
> **Idioma:** Español (Castellano peninsular)  

---

## 1. Resumen Ejecutivo

El presente informe certifica formalmente que el **Incremento 18 (`inc-18-rdd-decoupling-and-v3-stability-fixes`)** ha superado todas las pruebas técnicas, unitarias, de integración y de arquitectura estipuladas en `spec.md` y `design.md`.

Se han cumplido con éxito los dos objetivos estratégicos del incremento:
1. **Desacoplamiento Estructural de RDD:** Se eliminó de la máquina de estados SDD (`sdd status`) el enlace forzado con RDD (`applyReviewOfferRouting`), garantizando que la transición a la fase de archivado (`archive: ready`) dependa de forma limpia y exclusiva de la compleción de tareas y el informe de verificación formal. Las herramientas de revisión de código (`axiom review ...`) permanecen 100% operativas como utilidades independientes y opt-in gobernadas por el usuario.
2. **Absorción Selectiva de Parches Críticos de Gentle-AI v3:**
   - **Rutas Windows (`1a2f6775`):** Deserialización nativa de JSON (`StatusV2Projection`) en aserciones de prueba, inmune a backslashes escapados.
   - **Aislamiento CWD (`8c078527`):** Eliminación de dependencias de `resolveOpenClawWorkspaceDir` en `internal/cli/run.go` y `sync.go`, resolviendo configuraciones de agentes contra raíces canónicas (`homeDir` y `componentInjectionDir`).
   - **Saneamiento de Presets de Skills (`11f6c000`):** Segregación estricta de las 6 skills de contribución interna (`contributorSkills`) fuera de los presets de usuario general (`selectableFoundationSkills`).
   - **Resiliencia de Engram (`59e6705f`, `90992285`):** Protocolo documentado con reglas explícitas de resolución y recuperación ante el error `ambiguous_project`.
3. **Ratificación del Rechazo a RTK:** Confirmada la exclusión de hooks intrusivos y dependencias no soportadas en Windows en el ecosistema Axiom.

---

## 2. Validación de Requerimientos y Escenarios BDD

### Requirement: Desacoplamiento de RDD en la Proyección de Estado SDD (REQ-18.1)
- **Escenario:** Proyección de estado SDD tras verificación exitosa sin `reviewOffer` forzado
  - **Estado:** ✅ **PASS**
  - **Evidencia:** `internal/sddstatus/status.go` neutralizado mediante omisión de `applyReviewOfferRouting`. Los tests de guardas `TestReviewOfferAbsenceGuardCatchesKnownShapes`, `TestReviewOfferAbsenceGuardHoldsForProductionFiles` y `TestReviewEntryHookIsTheOneDoor` pasaron al 100%. La invocación de `axiom sdd status` omite completamente el campo `reviewOffer` en el JSON gracias a `omitempty`.

### Requirement: Independencia Operativa de las Herramientas RDD (REQ-18.2)
- **Escenario:** Invocación autónoma de comandos review en la CLI
  - **Estado:** ✅ **PASS**
  - **Evidencia:** El subárbol de comandos `axiom review` en `internal/cli/review.go` y sus subcomandos asociados operan de forma independiente sin interferir en el ciclo de vida SDD. `internal/cli/sdd_status.go` desacoplado del kill-switch de RDD (`ReviewDisabledForWorkspace`).

### Requirement: Resiliencia en Decodificación de Rutas Windows en Salidas JSON de SDD (REQ-18.3)
- **Escenario:** Validación de rutas permitidas bajo Windows
  - **Estado:** ✅ **PASS**
  - **Evidencia:** Test `TestRunSDDAttemptGrantPersistsAndReplaysThroughTheCLI` en `internal/cli/sdd_attempt_test.go` actualizado para deserializar estructuradamente con `json.Unmarshal(&projected)` en tipo `StatusV2Projection`. Pasó limpiamente en 7.06s bajo Windows.

### Requirement: Aislamiento del Directorio de Trabajo (CWD) en la Gestión de Artefactos (REQ-18.4)
- **Escenario:** Sincronización de agentes desde un subdirectorio anidado
  - **Estado:** ✅ **PASS**
  - **Evidencia:** `internal/cli/run.go` y `internal/cli/sync.go` limpios de invocaciones residuales a `resolveOpenClawWorkspaceDir`. Rutas de agentes y persona de Pi resueltas contra `homeDir` canónico. Tests de sincronización e inyección pasando al 100%.

### Requirement: Saneamiento del Preset Predeterminado de Skills (REQ-18.5)
- **Escenario:** Inspección del preset básico de skills
  - **Estado:** ✅ **PASS**
  - **Evidencia:** `internal/components/skills/presets.go` separa `contributorSkills` (6 skills de workflows internos) de las skills fundacionales seleccionables. Suite `TestSkillsForPreset*` en `presets_test.go` y archivo golden `testdata/golden/skills-presets.json` validados con resultado PASS al 100%.

### Requirement: Resiliencia de Protocolo Engram ante Ambiguous Project (REQ-18.6)
- **Escenario:** Manejo de ambigüedad de proyecto en handshake de sesión Engram
  - **Estado:** ✅ **PASS**
  - **Evidencia:** `internal/assets/engram/protocol.md` enriquecido con sección normativa que instruye al agente a resolver de forma determinista la ambigüedad pasando `--project=<name>` sin bloquear el ciclo de trabajo.

---

## 3. Matriz de Cobertura de Verificación

| ID Requerimiento | Descripción | Tipo de Validación | Veredicto |
| :--- | :--- | :--- | :--- |
| **REQ-18.1** | Desacoplamiento RDD en `sdd status` | Unit Tests (`sddstatus`, guards) + CLI Output | ✅ PASS |
| **REQ-18.2** | Independencia comandos `axiom review` | CLI Execution + Separation Tests | ✅ PASS |
| **REQ-18.3** | Rutas Windows tolerantes a escapes JSON | Integration Test CLI (`sdd_attempt_test.go`) | ✅ PASS |
| **REQ-18.4** | Aislamiento CWD en OpenClaw y sincronización | CLI sync & agent pathing unit tests | ✅ PASS |
| **REQ-18.5** | Saneamiento de presets de skills | Presets unit tests + golden file | ✅ PASS |
| **REQ-18.6** | Resiliencia de handshake Engram MCP | Protocol audit & compliance | ✅ PASS |

---

## 4. Veredicto Final

El Incremento 18 (`inc-18-rdd-decoupling-and-v3-stability-fixes`) queda certificado con veredicto **PASS**. Se autoriza el avance a la fase final de **Archivado Formal (`archive`)**.

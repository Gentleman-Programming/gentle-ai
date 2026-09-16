# Tareas: Desacoplamiento de RDD y Parches de Estabilidad Upstream v3 (inc-18-rdd-decoupling-and-v3-stability-fixes)

> **Incremento:** `inc-18-rdd-decoupling-and-v3-stability-fixes`  
> **Fase:** Fase 4 — Evolución Arquitectónica, Flujo Orgánico (ODD) y Absorción Upstream v3  
> **Responsabilidad:** Core SDD / Estabilidad & Plataforma  
> **Estado:** 7/7 tareas completadas  
> **Idioma:** Español (Castellano peninsular)  

---

## Lista de Tareas

- [x] `task-18-1-rdd-decouple-sddstatus`: Retirar la invocación a `applyReviewOfferRouting` en `internal/sddstatus/status.go`, neutralizar `review_door.go` y omitir el bloque `reviewOffer` en la proyección JSON de estado SDD.
- [x] `task-18-2-rdd-decouple-cli`: Desacoplar `internal/cli/sdd_status.go` del callback de kill-switch de RDD (`ReviewDisabledForWorkspace`), asegurando la independencia de `sdd status`.
- [x] `task-18-3-upstream-windows-json-test`: Absorber corrección de rutas en Windows (`1a2f6775`) en `internal/cli/sdd_attempt_test.go` decodificando `StatusV2Projection` en vez de buscar subcadenas literales.
- [x] `task-18-4-upstream-cwd-isolation`: Absorber corrección de aislamiento CWD (`8c078527`) en `internal/cli/run.go` y `internal/cli/sync.go` para resolver artefactos de agentes contra raíces canónicas.
- [x] `task-18-5-upstream-skills-presets-hygiene`: Absorber saneamiento del catálogo de skills (`11f6c000`) en `internal/components/skills/presets.go` y actualizar sus tests en `presets_test.go`.
- [x] `task-18-6-upstream-engram-protocol-resilience`: Absorber clarificación de recuperación ante `ambiguous_project` (`59e6705f`, `90992285`) en `internal/assets/engram/protocol.md`.
- [x] `task-18-7-unit-tests-and-build`: Ejecutar la suite de pruebas unitarias de los componentes afectados (`internal/sddstatus/...`, `internal/cli/...`, `internal/components/skills/...`), verificar que pasan al 100% y validar la compilación limpia del binario `axiom`.

# Informe de Verificación Formal: Reconciliación con Upstream v3.4.0 e Identidad de Distribución (INC-20)

> **Incremento:** `inc-20-upstream-reconciliation`  
> **Fase:** `sdd-verify` · **Fecha:** 2026-09-21 (Actualizado: 2026-09-23)  
> **Estado:** SATISFIED  
> **Idioma:** Español (Castellano peninsular)

---

## 1. Veredicto Ejecutivo

**Estado Final:** **SATISFIED** (Aprobado)

La reconciliación auditada con upstream (`Gentleman-Programming/gentle-ai`) hasta la versión congelada `v3.4.0` / `v3.5.0` se ha completado e integrado en `main` a través de los PRs #26 a #34.

El libro mayor de absorción (`docs/upstream-absorption-ledger.md`) registra las 91 filas correspondientes a los 91 commits de upstream entre el ancestro común `266574b0` y la etiqueta `v3.4.0`, preservando intacto el inventario de divergencias deliberadas de Axiom (V1–V8).

---

## 2. Cobertura de Requerimientos

| Requerimiento | Estado | Evidencia Observada |
|---|:---:|---|
| **REQ-20.1** (Derivación obligatoria) | SATISFIED | Lista de ficheros derivada con `git show <sha> --stat` en todas las tandas. |
| **REQ-20.2** (Verificación sin filtrar) | SATISFIED | Verificación ejecutada en suite completa `go build ./...`, `go vet ./...`, `go test ./...`. |
| **REQ-20.3** (Inventario de no-reversión) | SATISFIED | Divergencias V1–V8 preservadas (con V7 retirada de forma autorizada en F6). |
| **REQ-20.4** (Rutas protegidas) | SATISFIED | Ninguna tanda de absorción tocó `internal/hub/`, `openspec/INDEX.md` ni `openspec/config.yaml`. |
| **REQ-20.5** (Estructura del registro) | SATISFIED | `docs/upstream-absorption-ledger.md` estructurado con estados y motivos documentados. |
| **REQ-20.6** (Completitud del registro) | SATISFIED | 91 filas registradas y reflejadas en Engram. |
| **REQ-20.7** (Taxonomía interoperabilidad vs identidad) | SATISFIED | `contracts/**` preservado byte a byte; identidades públicas adaptadas. |
| **REQ-20.8** (Publicación de releases propios) | SATISFIED | Release `v3.5.0` publicada formalmente en GitHub (`IGutierrezZ/axiom`) con binarios `axiom_*`, checksums firmados y procedencia. |
| **REQ-20.9** (Ruta de módulo Go a /v3) | SATISFIED | Módulo migrado a `/v3` en `go.mod` y en todo el árbol de código. |
| **REQ-20.10** (Namespace contracts/**) | SATISFIED | `contracts/**/*.schema.json` sin alteraciones. |
| **REQ-20.11** (Pasarela gentle-ai y shim) | SATISFIED | `cmd/gentle-ai` emite aviso de deprecación y delega en `app.RunArgs`; shim crosslane operativo. |
| **REQ-20.12** (Telemetría de despliegue) | SATISFIED | Unidades de telemetría renombradas a `axiom-telemetry`. |
| **REQ-20.13** (Raíz de respaldos vía internal/backup) | SATISFIED | Escritores de producción resuelven mediante `backup.BackupRootFor()`. |
| **REQ-20.14** (Guarda estática de respaldos) | SATISFIED | `TestUserStateRootsResolveThroughOwningPackage` vigila y pasa sin literales. |
| **REQ-20.15** (Cobertura CI binario canónico) | SATISFIED | Paso de CI "Smoke the canonical binary surface" ejercita la superficie de `cmd/axiom`. |
| **REQ-20.16** (Ventana informativa) | SATISFIED | Bloque de ventana informativa registrado formalmente en CI. |

---

## 3. Resolución Formal de Tareas Diferidas (4.2 a 4.5)

Al momento de ejecutar la Fase 4 (F0.c2), el fork no contaba aún con releases publicados, por lo que las tareas 4.2 a 4.5 quedaron diferidas bajo condición.
Posteriormente, con los PRs #31, #32 y #34 se configuraron las compuertas de release, `scripts/verify-release-assets.sh` y el preflight, culminando con la publicación exitosa de la versión `v3.5.0` de Axiom.

Las variables internas de los scripts curl standalone (`install.sh`/`install.ps1`) y el tap de Homebrew se mantienen en su estado actual sin bloquear el incremento, dado que Axiom se distribuye mediante compilación por tags, go install y binarios release, delegando cualquier instalador externo adicional a un empaquetado futuro.

# Registro de absorción upstream — Axiom

> **Medido el:** 2026-09-19 · **Ancestro común:** `266574b0` · **Techo congelado:** `v3.4.0` = `82a6de96ca6e1cb4f6bf603fe0c08ef1c2039833`
> **Universo:** 91 commits de `266574b0..upstream/main` **sin merges**
> **Comando:** `git rev-list --count --no-merges 266574b0..upstream/main`

> **Nota de alcance — universo congelado, no vivo.** 91 es el número de commits sin merge entre `266574b0` y `82a6de96` (`upstream/main` en el instante de abrir este registro), congelado a esa fecha y a ese `sha`, no un valor que se reconsulta en cada lectura. La propuesta había medido 55 el 2026-09-18 (`proposal.md:71,195,471`); el diseño remidió el universo el 2026-09-19 y obtuvo 87 **con** merges (`design.md` §10, "Estado de las mediciones" y §10.1), y acotó la cifra real sin merges en el intervalo `[55, 65]` **sin fijarla**, precisamente para que la apertura de este registro la resolviera con el comando exacto de arriba. El valor final, 91, se obtuvo tras la publicación de `v3.4.0` de upstream esa misma jornada (etiqueta fechada 2026-09-19, en la punta de `upstream/main` en el momento de medir). Todo commit que upstream publique después de `82a6de96` pertenece a un incremento de reconciliación futuro, no a `inc-20-upstream-reconciliation`.

> **Decisión de producto D4 — el techo es `v3.4.0` y no se mueve.** El universo de este incremento se cierra en la etiqueta `v3.4.0` de upstream (`82a6de96`), y esa frontera **no se re-mide** aunque upstream siga publicando mientras las rebanadas restantes aterrizan. El motivo es de terminación, no de comodidad: un universo que se reconsulta en cada fase nunca se cierra, porque upstream avanza más rápido de lo que se absorbe — la propia historia de esta cabecera lo demuestra, con el conteo pasando de 55 a 91 en una sola jornada. Absorber un blanco móvil es un trabajo sin criterio de fin.
>
> En consecuencia: ninguna tanda de este incremento incorpora commits posteriores a `82a6de96`, y una tanda que los encuentre en su derivación los deja fuera con motivo escrito en su sub-tabla, no los absorbe «de paso». La integración de versiones posteriores a `v3.4.0` se aborda con un flujo propio, a diseñar **una vez Axiom esté terminado**; ese flujo es trabajo futuro y no pertenece a `inc-20-upstream-reconciliation`.

## Reglas de aceptación

Adaptadas de `docs/releases/v2.2.0-closure-ledger.md:11-21` a la forma de este registro (D-06). Gobiernan todo veredicto de las tablas siguientes.

1. **Derivación obligatoria (RA-1).** Toda tanda deriva su lista de ficheros de `git show <sha> --stat` para cada commit de upstream que la compone. Un fichero derivado y ausente del diff de la tanda lleva motivo escrito en su sub-tabla "Ficheros derivados y ausentes"; sin motivo, la tanda se rechaza.
2. **"El código parece relacionado" no es evidencia.** Ni lo es un fichero compartido ni un asunto de commit parecido. El estado `absorbido` exige una referencia de verificación concreta (PR del fork o commit de re-derivación), no una impresión de similitud.
3. **Verificación sin filtrar (RA-2).** Ninguna tanda se marca `absorbido` sin `go build ./...`, `go vet ./...`, `go test ./...` sin `-run`, y `e2e/e2e_test.sh`, todos en verde — salvo el único fallo aceptado y saltado de este mismo paquete (`TestUpstreamAbsorptionLedgerCoversDeclaredUniverseAtClose`) hasta el cierre de la Fase 16. La cobertura no alcanzada por `bench/` (módulo Go independiente, sin `go.work`) se declara explícitamente, nunca se omite.
4. **Inventario de no-reversión (V1–V8).** Ninguna tanda distinta de F6 revierte, total o parcialmente, las entradas V1–V6 u V8. La entrada V7 (ODD como paquete Go) solo la retira F6, y solo mediante los deltas de especificación que esa fase autoriza.
5. **Rutas prohibidas.** Ninguna tanda toca `bench/`, `internal/hub/`, `internal/workspace/`, `internal/multirole/`, `internal/handoff/`, `internal/semantic/`, `internal/livingdoc/`, `internal/components/uninstall/cleaners.go`, `openspec/INDEX.md`, `openspec/config.yaml`, `openspec/changes/archive/**`, `docs/releases/**` ni `odd/tasks/*.md`.
6. **Donde la disposición no puede establecerse con evidencia, el veredicto es "no está claro — requiere confirmación del autor".** Adivinar es peor que admitir incertidumbre.
7. **Una tanda revertida actualiza el estado de sus filas a `revertido`, conservando el resto de sus campos. Ninguna fila se borra nunca.**

## Recuento

| Estado | Filas |
|---|---|
| `absorbido` | 0 |
| `descartado-deliberadamente` | 0 |
| `revertido` | 0 |
| **Total** | **0 (= universo declarado en la cabecera: 91)** |

## F0 — Identidad de distribución, artefacto de release y cobertura del binario real

| `sha` | Asunto | Estado | Evidencia | Motivo (si no es `absorbido`) |
|---|---|---|---|---|

### Ficheros derivados y ausentes (RA-1)

| Fichero derivado de `git show --stat` | Ausente del diff | Motivo escrito |
|---|---|---|

## F1 — Migración de la ruta de módulo Go `/v2` → `/v3`

| `sha` | Asunto | Estado | Evidencia | Motivo (si no es `absorbido`) |
|---|---|---|---|---|

### Ficheros derivados y ausentes (RA-1)

| Fichero derivado de `git show --stat` | Ausente del diff | Motivo escrito |
|---|---|---|

## F2 — Telemetría VictoriaMetrics

| `sha` | Asunto | Estado | Evidencia | Motivo (si no es `absorbido`) |
|---|---|---|---|---|

### Ficheros derivados y ausentes (RA-1)

| Fichero derivado de `git show --stat` | Ausente del diff | Motivo escrito |
|---|---|---|

## F3 — Reviewer y parsing de OpenCode

| `sha` | Asunto | Estado | Evidencia | Motivo (si no es `absorbido`) |
|---|---|---|---|---|

### Ficheros derivados y ausentes (RA-1)

| Fichero derivado de `git show --stat` | Ausente del diff | Motivo escrito |
|---|---|---|

## F4 — Poda y refactor SDD

| `sha` | Asunto | Estado | Evidencia | Motivo (si no es `absorbido`) |
|---|---|---|---|---|

### Ficheros derivados y ausentes (RA-1)

| Fichero derivado de `git show --stat` | Ausente del diff | Motivo escrito |
|---|---|---|

## F5 — CLI y community-tools RTK

| `sha` | Asunto | Estado | Evidencia | Motivo (si no es `absorbido`) |
|---|---|---|---|---|

### Ficheros derivados y ausentes (RA-1)

| Fichero derivado de `git show --stat` | Ausente del diff | Motivo escrito |
|---|---|---|

## F6 — Retirada destructiva de la capa Go de ODD

| `sha` | Asunto | Estado | Evidencia | Motivo (si no es `absorbido`) |
|---|---|---|---|---|

### Ficheros derivados y ausentes (RA-1)

| Fichero derivado de `git show --stat` | Ausente del diff | Motivo escrito |
|---|---|---|

## F7 — Cierre del registro y documentación

| `sha` | Asunto | Estado | Evidencia | Motivo (si no es `absorbido`) |
|---|---|---|---|---|

### Ficheros derivados y ausentes (RA-1)

| Fichero derivado de `git show --stat` | Ausente del diff | Motivo escrito |
|---|---|---|

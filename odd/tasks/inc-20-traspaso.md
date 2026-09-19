# INC-20 — Traspaso de sesión (2026-09-19)

> **Documento de continuidad**, no un artefacto SDD. Los artefactos autoritativos del incremento están en `openspec/changes/inc-20-upstream-reconciliation/`.

## Dónde está el incremento

`inc-20-upstream-reconciliation`: reconciliación con upstream (`Gentleman-Programming/gentle-ai`) e identidad de distribución de Axiom. Planificación **completa y fusionada**; implementación en la rebanada 2 de 17.

| Fase | Estado |
|---|---|
| Propuesta, especificación, diseño, tareas | ✅ Fusionadas en `main` (PR #24, `9b23cf01`) |
| Rebanadas 1 y 2 (F0.a + F0.b) | PR #26, unidas, con `size:exception` |
| Rebanadas 3 a 17 | Sin empezar |

**154 tareas en 17 rebanadas**, cadena `stacked-to-main`. Preflight de sesión fijado: ritmo `auto`, artefactos `hybrid`, entrega `auto-chain`.

## Lo primero al retomar

1. **Comprobar PR #26.** Si está en verde, fusionarlo. Si no, leer el fallo antes de tocar nada.
2. **Lanzar la rebanada 3 (F0.c1)** — identidad de build, cobertura de CI, REQ-20.15 y REQ-20.16.
3. Antes de abrir PRs: `git log origin/main..main` y `gofmt -l .`. Las dos han mordido antes.

## Decisiones del usuario ya cerradas — no reabrir

- **D1**: ODD se adopta de upstream de forma **destructiva**. No se conserva nada de la capa Go propia del fork.
- **D2**: identidad de distribución resuelta en todo el inventario de contrato publicado.
- **D3**: se mantiene la relación con upstream.
- **Cadena**: `stacked-to-main`.
- **`size:exception`**: aceptada para las rebanadas 14 y 17 (medidas), más #24 (documentos) y #26 (puertas incompatibles). Cualquier otra, preguntar.

## Bloqueado por el mantenedor, no por código

**El tramo de instalador de la rebanada 4** depende de REQ-20.8, que exige un release publicado bajo la identidad de Axiom. Medido: **el fork tiene cero releases**; upstream va por `v3.3.0`. El resto de esa rebanada avanza igual.

## Hallazgos vivos que afectan a rebanadas futuras

### `.goreleaser.yaml` publica el shim deprecado

`.goreleaser.yaml:13-14` construye `main: ./cmd/gentle-ai`, que es un shim de 27 líneas que delega en `app.RunArgs`. Ese despachador **no tiene caso** para `odd`, `init`, `change`, `workspace`, `handoff`, `role`, `skill`, `semantic`, `archive` ni `ui`. El binario que Axiom publicaría no sabe ejecutar los comandos propios del fork. Rebanada 3.

### `axiom archive sync` muta un fichero versionado

Regenera `openspec/INDEX.md` con una marca de tiempo nueva **en cada ejecución** (`internal/livingdoc`). Cualquier paso de CI que ejercite la superficie exclusiva —que es justo lo que REQ-20.15 obliga— ensucia el árbol de forma no determinista. Rebanada 3 debe resolverlo.

### El catálogo maestro está desincronizado

`openspec/INDEX.md` declaraba 8 escenarios para `dashboard-sdd-orchestration`, que tiene 10. Hay un chip de tarea abierto para resincronizarlo **fuera de este incremento** (es ruta prohibida).

### Una rebanada aditiva sin consumidor no es entregable aquí

Lo aprendido al fusionar #25 en #26: el trinquete de código muerto rechaza una función sin llamador, así que una rebanada estrictamente aditiva no puede aterrizar sola. Afecta a la planificación de las quince restantes.

### La guarda de raíces no ve fuera de Go

`TestUserStateRootsResolveThroughOwningPackage` analiza árboles sintácticos de Go. Los literales en `e2e/*.sh` le son invisibles — así se escapó el decimonoveno sitio. Cualquier auditoría futura de literales debe cubrir shell aparte.

## Entorno de verificación

Contenedor Docker `axiom-bench`, repo montado en `/repo`, cachés calientes.

```bash
docker exec axiom-bench sh -c 'cd /repo && go build ./... && go vet ./...'
```

Usuario sin privilegios, que se parece más al CI:

```bash
MSYS_NO_PATHCONV=1 docker exec -u axiomtest \
  -e GOCACHE=/home/axiomtest/.cache/go-build \
  -e GOMODCACHE=/home/axiomtest/go/pkg/mod \
  axiom-bench sh -c 'cd /repo && go test ./...'
```

**Tiene techo, y conviene saberlo.** Como root se invalidan los tests que simulan «permiso denegado»; como no-root, el producto rechaza `/repo/.git` por pertenecer a uid 0, que es como Docker Desktop monta en Windows. **Ninguno de los dos reproduce el CI del todo.** Sirve para build, vet y la mayoría de paquetes; para la suite completa el árbitro es el CI.

El E2E de respaldos **no corre por defecto**: pide `RUN_BACKUP_TESTS=1`. El CI sí lo activa (`ci.yml:525`).

Hay un `axiom.exe` de Windows en la raíz, ignorado por git, y `cmd/axiom/main_test.go:313` codifica ese nombre. Al correr `go test ./cmd/...` en Linux sobre el montaje da `exec format error`: apártalo y devuélvelo.

## Trampa de entrega, ya pagada una vez

Fusionar con `--delete-branch` un PR que es **base** de otro **cierra el PR hijo** en vez de reapuntarlo, y queda atrapado. Método correcto: `gh pr edit N --base main` antes de fusionar cada uno, y **nunca `--delete-branch`** durante la cadena.

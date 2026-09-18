# Informe de Verificación: inc-19-odd-workflow-and-promotion

**Fase**: `sdd-verify` · **Fecha**: 2026-09-18 · **Alcance**: cadena de 15 rebanadas apiladas, 84/84 tareas

## 0. Nota de procedencia

Durante la ejecución de esta fase, el orquestador conmutó el árbol de trabajo compartido a otra rama (`ci/desactivar-puerta-issue-first`) para resolver un bloqueo de CI, mientras el verificador seguía operando en el mismo directorio. El verificador lo detectó cruzando `git reflog --date=iso` contra las marcas de tiempo de sus propios comandos, y actuó correctamente:

- Las ocho comprobaciones funcionales se habían ejecutado **antes** de la conmutación, sobre la rama correcta. Su evidencia es válida y se recoge en §2.
- Una inspección estática posterior de `internal/tui/model.go`, que parecía mostrar `ScreenODDFeatures` ausente, se hizo sobre la rama equivocada y **queda descartada**. La evidencia de test previa (`TestScreenOptionCountMatchesODDFeaturesOptions`, `TestODDFeaturesScreenPromotedFeatureJumpsToSDDLane`, ambas en verde) la contradice y prevalece.
- El verificador se negó a escribir este fichero mientras el árbol estaba en una rama ajena, para no arrastrarlo a un commit no relacionado. Fue la decisión correcta; este documento se escribe ya sobre la rama del incremento.

La conmutación fue un error de coordinación del orquestador, no un defecto del incremento ni una actuación indebida del verificador.

## 1. Resumen ejecutivo

- **Tareas**: 84/84 completas, consistente con `apply-progress`. Ninguna casilla reescrita.
- **Build y vet**: `go build ./...` y `go vet ./...` limpios, salida vacía, código de salida 0.
- **Ocho comprobaciones funcionales acotadas**: todas en verde (§2).
- **Suite completa**: **no está en verde y no se declara como tal** (§3).
- **Cobertura**: 27 de 37 escenarios con test automático nombrado, 4 parciales, 4 solo por inspección, 2 huecos. Los 16 requerimientos tienen al menos un escenario con prueba automática (§4).
- **Higiene**: `git status --porcelain` limpio tras cada comprobación.

## 2. Comprobaciones funcionales

| # | Comando | Resultado |
|---|---|---|
| 1 | `go build ./...` | PASS, sin salida |
| 2 | `go vet ./...` | PASS, sin salida |
| 3 | `go test ./internal/odd/...` | PASS — 28 funciones de test, 0 fallos |
| 4 | `go test ./internal/dashboard/ -run 'TestODD\|TestNameParity\|TestCreateIncrement'` | PASS — 6 funciones, 0 fallos |
| 5 | `go test ./internal/tui/...` | PASS — 628 líneas `--- PASS`, 0 fallos |
| 6 | `go test ./internal/cli/ -run 'TestRunODD\|TestODD'` | PASS — 0 fallos |
| 7 | `go test ./cmd/axiom/... -run 'TestRunODD\|TestRunChange'` | PASS — 7 funciones, 0 fallos |
| 8 | `go test ./internal/assets/...` | PASS — 113 líneas `--- PASS`, incluidos los 7 subcasos de la Persona `axiom` |

Verificaciones adicionales del orquestador con binarios reales, no por informe de terceros:

- `axiom odd create|status|promote` funcionan de extremo a extremo. La promoción siembra la propuesta, marca el documento y rechaza una segunda promoción; `--dry-run` no escribe nada.
- La Web UI renderiza los documentos con progreso y marca de promovido, y la conmutación es bidireccional. Comprobado en navegador sobre un workspace de prueba.
- La TUI expone la pantalla desde Gobernanza, con recorrido real y «Volver» funcionando desde su nueva posición.
- **REQ-15.1 preservado byte a byte**: el `proposal.md` que generan el binario de `main` y el de esta rama con los mismos argumentos tiene SHA-256 idéntico.

## 3. Por qué la suite completa no está en verde

`openspec/config.yaml` pide `go test ./...` como comprobación de fase, pero la línea base de este repositorio ya está en rojo en **diez paquetes** por causas ajenas a este incremento, verificadas de forma independiente contra `main`: privilegio de symlink ausente en Windows (causa dominante), fixtures obsoletas del renombrado `gentle-ai`→`axiom`, dependencia del estado de la máquina, y el trinquete de rehúses.

**El código de salida de `go test ./...` no es 0, y este informe no declara que lo sea.** Fabricar un cero en el envoltorio de verificación formal sería falsificar evidencia. La evidencia funcional honesta de este incremento son las ocho comprobaciones acotadas de §2, todas en verde, más las verificaciones con binarios reales.

Durante el cierre de la Fase 8 sí se ejecutó la suite completa una vez: 92 paquetes, 79 en verde, 10 en rojo, comparados uno a uno contra el inventario conocido. **Cero fallos nuevos atribuibles al incremento.**

## 4. Cobertura de requerimientos y escenarios

Leyenda: **A** prueba automática nombrada y observada en verde · **P** parcial · **I** solo inspección o garantía estructural · **H** hueco sin evidencia automática.

| Capacidad | A | P | I | H |
|---|---|---|---|---|
| `odd-living-document` (REQ-19.1 – 19.4) | 7 | 0 | 1 | 0 |
| `odd-cli-commands` (REQ-19.5 – 19.8) | 8 | 1 | 0 | 1 |
| `odd-sdd-promotion` (REQ-19.9 – 19.12) | 8 | 0 | 1 | 0 |
| `odd-ui-integration` (REQ-19.13 – 19.15) | 1 | 3 | 2 | 0 |
| `dashboard-sdd-orchestration` (REQ-15.1) | 3 | 0 | 0 | 1 |
| **Total (37 escenarios)** | **27** | **4** | **4** | **2** |

### Los dos huecos

1. **REQ-19.8 — el grupo `odd` aparece en `axiom --help`.** `TestRunODDHelp` ejercita la ayuda propia de `runODD` (`axiom odd --help`), que es otra superficie; `printHelp()` solo estaba comprobado a mano. **Cerrado en esta misma rebanada** con `TestPrintHelpAnunciaElGrupoODD`, que además fija que ningún subcomando quede marcado como no disponible.
2. **REQ-15.1 — rechazo ante nombre inválido o colisión.** Comportamiento preexistente, no tocado por el diseño (`ProposalBody` vacío ejecuta literalmente el flujo anterior, fijado por el test de caracterización). Muy probablemente cubierto por tests anteriores de `internal/dashboard` fuera del filtro empleado. No se declara roto: se declara sin nombre de test propio de esta sesión.

### Las cuatro coberturas parciales

Se concentran en la superficie de interfaz: el frontend de la Web UI es JavaScript sin arnés de test automatizado en el proyecto, y su verificación fue manual con navegador headless, que detectó y corrigió dos defectos reales. Los renderizadores de la TUI viven en un paquete servido desde caché en la sesión de verificación, lo que impide citar el nombre exacto del test por escenario aunque el paquete esté en verde.

## 5. Desviaciones conocidas, contrastadas y consistentes

Seis desviaciones documentadas durante la implementación se contrastaron sin reabrirlas como hallazgos nuevos: la enmienda de D-01 sobre el ciclo de importación, la firma de cuatro parámetros de `CheckMirror`, `PromoteResult.Body` con `json:"-"`, el espejo Engram sin filtro por proyecto, las tres acciones de la pantalla TUI que señalan el comando de la CLI, y el enrutado roto preexistente de `ScreenSDDIncrements`.

## 6. Veredicto

La implementación se corresponde con la especificación y el diseño. No hay defectos funcionales confirmados ni regresiones atribuibles al incremento. Lo que queda son huecos de evidencia de verificación, uno de ellos cerrado en esta misma rebanada, y una cobertura de interfaz que depende de verificación manual por ausencia de arnés automatizado de frontend en el proyecto.

Ninguno de estos hallazgos exige volver a `sdd-apply`.

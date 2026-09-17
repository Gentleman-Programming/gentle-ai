# Diseño: Flujo ODD y Promoción a SDD (inc-19-odd-workflow-and-promotion)

> **Incremento:** `inc-19-odd-workflow-and-promotion`
> **Fase:** `sdd-design` · **Fecha:** 2026-09-17
> **Fuente de alcance:** `openspec/changes/inc-19-odd-workflow-and-promotion/proposal.md`
> **Idioma:** Español (castellano peninsular). Identificadores Go en inglés; valores serializados visibles al usuario, en castellano.

---

## 0. Resumen del diseño

| Pregunta | Respuesta |
|---|---|
| ¿Cuál es la pieza central? | Un paquete hoja `internal/odd` sin dependencias hacia `dashboard`, `cli` ni `tui`. Todo lo demás son adaptadores finos. |
| ¿Cómo se evita el ciclo de importación? | `internal/dashboard` ya importa `internal/cli`. Si `internal/odd` importase `dashboard`, el ciclo `cli → odd → dashboard → cli` sería inevitable. Se resuelve con un **puerto** `odd.Scaffolder`, implementado por un adaptador en `internal/cli/odd_promote.go`. |
| ¿Cómo se protege REQ-15.1? | Campo **opcional** `ProposalBody` en `CreateIncrementRequest`. Rama única de tres líneas. Test de caracterización con los bytes exactos de la plantilla vigente, escrito **antes** de tocar la función. |
| ¿Cómo se detecta la divergencia? | Sondeo de solo lectura `engram export` con *timeout* acotado, detrás de un puerto `odd.Exporter` inyectable. Tres estados, ningún error, ningún cambio de código de salida. |
| ¿Cómo se conmuta de carril? | Web UI: pestaña `tab-odd` + enlaces cruzados resueltos en cliente. TUI: nueva pantalla `ScreenODDFeatures` con acciones tipadas (no aritmética de índices). |
| ¿Qué NO toca este diseño? | `internal/sddstatus/`, `internal/cli/sdd_*.go`, `internal/agents/researchcapability/`, `openspec/INDEX.md`, `openspec/config.yaml`. |

---

## 1. Enfoque Técnico (Technical Approach)

### 1.1 Forma general

El incremento introduce **un dominio nuevo y tres adaptadores**. El dominio (`internal/odd`) conoce el documento vivo, su gramática, su progreso y su traducción a propuesta SDD. No conoce HTTP, ni Bubbletea, ni `flag`, ni Engram, ni el andamiador de incrementos. Los tres adaptadores (CLI, Web UI, TUI) lo consumen y le inyectan lo que necesita del exterior.

Esta forma no es una preferencia estética: es la **única** que evita un ciclo de importación real, verificado en el árbol.

```
                       stdlib + internal/multirole
                                   ▲
                                   │
                          ┌────────┴────────┐
                          │   internal/odd  │  ← paquete hoja, dominio puro
                          │  (sin salidas)  │
                          └────────┬────────┘
                    ┌──────────────┼──────────────┐
                    │              │              │
           ┌────────┴──────┐ ┌─────┴────────┐ ┌───┴──────────┐
           │  internal/cli │ │  internal/   │ │ internal/tui │
           │  odd_*.go     │ │  dashboard   │ │ screens/odd  │
           └───────┬───────┘ └──────┬───────┘ └───┬──────────┘
                   │                │             │
                   └────────────────┤             │
                     (dashboard ya importa cli)   │
                                    │             │
                              cmd/axiom/main.go ──┘
```

**Aristas nuevas** (todas seguras, verificado que `internal/tui` importa `internal/cli` pero **no** `internal/dashboard`):

| Arista | ¿Existe hoy? | ¿Introduce ciclo? |
|---|---|---|
| `internal/odd` → `internal/multirole` | nueva | No — `multirole` es hoja |
| `internal/cli` → `internal/odd` | nueva | No |
| `internal/dashboard` → `internal/odd` | nueva | No |
| `internal/tui` → `internal/odd` | nueva | No |
| `internal/odd` → *cualquier cosa de Axiom salvo `multirole`* | **prohibida** | Sí, provocaría `cli → odd → dashboard → cli` |

### 1.2 Correspondencia con la propuesta

| Principio rector (§4.1 de la propuesta) | Materialización en este diseño |
|---|---|
| Aditivo antes que invasivo | 5 ficheros modificados de producción frente a ~13 nuevos. La modificación de `CreateIncrement` es una rama de 3 líneas. |
| El fichero manda; Engram es espejo | `internal/odd` no tiene ninguna función de escritura hacia Engram. El puerto `Exporter` es de solo lectura, por firma. |
| Reutilizar lo que ya funciona | `multirole.CountTasks` para el progreso; `dashboard.Service.CreateIncrement` a través del puerto `Scaffolder`, con su validación de nombre y sus dos guardas de colisión intactas. |
| La promoción transporta lo conocido; nunca fabrica rigor | El renderizador emite un marcador explícito en `## Capacidades` y `_(sin contenido en el documento ODD de origen)_` en cada sección vacía. |

---

## 2. Decisiones de Arquitectura (Architecture Decisions)

### D-01 — `internal/odd` es un paquete hoja; la promoción entra por un puerto

**Elección.** `internal/odd` no importa ningún paquete de Axiom salvo `internal/multirole`. La creación del cambio SDD se expresa como una interfaz consumida por el dominio:

```go
type Scaffolder interface {
    Scaffold(ScaffoldRequest) (ScaffoldResult, error)
}
```

El adaptador que la satisface con `dashboard.Service.CreateIncrement` vive en `internal/cli/odd_promote.go`.

**Alternativas descartadas.**

1. *`internal/odd` importa `internal/dashboard` directamente.* Descartada por **imposible**: `internal/dashboard/service.go:864` llama a `cli.RunSDDContinue`, luego `dashboard → cli`. Como `internal/cli/odd_promote.go` debe importar `odd`, el grafo cerraría el ciclo `cli → odd → dashboard → cli` y el compilador lo rechazaría. No es una preferencia arquitectónica, es una restricción del árbol actual.
2. *Toda la lógica de promoción en `internal/cli/odd_promote.go`, importando `odd` y `dashboard`.* Compila y no cicla, pero deja la secuencia de dos fases (crear cambio → marcar documento), su orden y su comportamiento ante fallo parcial fuera del alcance de un test unitario del dominio: sólo podría probarse escribiendo en disco a través del servicio real. El puerto permite una tabla de casos con un `Scaffolder` falso que devuelve colisión, error de E/S o éxito, sin tocar el sistema de ficheros de `openspec/`.
3. *Mover `CreateIncrement` a un paquete neutro compartido.* Toca `internal/dashboard` mucho más de lo necesario y arriesga REQ-15.1 sin contrapartida. Contradice el principio «aditivo antes que invasivo».

**Justificación.** La opción elegida es la única que combina «no cicla», «reutiliza el andamiador existente en vez de duplicarlo» y «la secuencia crítica de dos fases es unitariamente comprobable».

---

### D-02 — `CreateIncrement` acepta un cuerpo ya renderizado; el dominio ODD posee sus bytes

**Elección.** Un campo opcional en la petición:

```go
type CreateIncrementRequest struct {
    Name         string `json:"name"`
    Intent       string `json:"intent"`
    Type         string `json:"type,omitempty"`
    ProposalBody string `json:"proposal_body,omitempty"` // INC-19: cuerpo ya renderizado (promoción ODD)
}
```

y una única rama insertada **entre** el `fmt.Sprintf` de la plantilla (`service.go:805-836`) y la escritura del fichero (`:838-841`):

```go
if strings.TrimSpace(req.ProposalBody) != "" {
    proposalContent = req.ProposalBody // bytes exactos del renderizador ODD, sin normalizar
}
```

`internal/dashboard` **no** sabe qué es una propuesta sembrada: recibe texto opaco y lo escribe. Todo el conocimiento del formato vive en `internal/odd`.

**Alternativas descartadas.**

1. *`ProposalBody` sustituye sólo ciertas secciones.* Exige que `dashboard` parsee Markdown y conozca la estructura de la propuesta. Multiplica las rutas de código que pueden alterar el resultado cuando el campo está vacío, que es justo lo que R3 prohíbe arriesgar.
2. *Un método hermano `CreateIncrementFromBody`.* Duplica las dos guardas de colisión (`service.go:776-788`) y la validación de nombre (`:770-772`), o las factoriza, lo que vuelve a tocar el camino de REQ-15.1. La deriva entre las dos copias sería cuestión de tiempo.
3. *`internal/odd` escribe `proposal.md` por su cuenta y sólo consulta al andamiador para validar.* Pierde la atomicidad de «crear el directorio y el fichero», duplica `os.MkdirAll` y contradice el §4.1.3 de la propuesta.
4. *Normalizar el cuerpo recibido (recortar, garantizar salto final).* Descartada: introduce una transformación en `dashboard` sobre bytes que el renderizador ya garantiza, y hace que dos capas puedan discrepar sobre el contenido final. El renderizador ODD es el único responsable del salto de línea terminal, y su test lo asegura.

**Justificación.** La rama es una asignación condicional sin efectos colaterales, posterior al cálculo de la plantilla. Cuando `ProposalBody` está vacío, el flujo ejecutado es **literalmente el mismo** que hoy, y el test de caracterización de D-03 lo demuestra byte a byte.

---

### D-03 — Test de caracterización de la plantilla **antes** de tocar `CreateIncrement`

**Elección.** En la rebanada P3, y como primera tarea de la rebanada, se escribe `internal/dashboard/create_increment_characterization_test.go` con el contenido exacto que `CreateIncrement` produce hoy para una entrada fija, comparado con `!=` sobre la cadena completa (no `strings.Contains`):

```go
func TestCreateIncrement_PlantillaVigenteSinCuerpoSembrado(t *testing.T) {
    tests := []struct {
        name string
        req  CreateIncrementRequest
        want string
    }{
        {name: "intento y tipo explícitos", req: CreateIncrementRequest{Name: "mi-cambio", Intent: "Propósito X", Type: "feature"}, want: plantillaEsperadaConIntento},
        {name: "intento vacío usa el texto por defecto", req: CreateIncrementRequest{Name: "mi-cambio"}, want: plantillaEsperadaSinIntento},
        {name: "tipo vacío no altera el cuerpo", req: CreateIncrementRequest{Name: "mi-cambio", Intent: "Propósito X"}, want: plantillaEsperadaConIntento},
    }
    // ... t.Run + os.ReadFile(res.Path) + comparación de la cadena íntegra
}
```

El literal esperado se transcribe del fichero generado por el binario **antes** del cambio, no de la lectura del `fmt.Sprintf`.

**Alternativas descartadas.**

1. *Fichero *golden* con `-update`.* Introduce maquinaria de *goldens* donde el repositorio no la tiene en este paquete, y un literal de ~32 líneas no la justifica. Además, un `-update` accidental borra exactamente la protección que se busca.
2. *Aserciones por subcadena (`strings.Contains`).* No detectan reordenaciones, pérdida de líneas en blanco ni cambios de espaciado — precisamente las regresiones silenciosas que produce editar un `fmt.Sprintf` de 32 líneas.
3. *Escribir el test después del cambio.* Incumple `strict_tdd: true` y, peor, caracterizaría el comportamiento **nuevo**, no el vigente: sería un test que no puede fallar por la regresión que pretende prevenir.

**Justificación.** El riesgo R3 tiene severidad **alta** y su mitigación declarada en la propuesta es exactamente ésta. El orden importa: un test de caracterización escrito después del cambio no caracteriza nada.

---

### D-04 — El progreso se calcula sobre la **rebanada del checklist**, no sobre el documento completo

**Elección.** `odd.Parse` localiza la sección «Checklist accionable», extrae su texto y sólo entonces invoca `multirole.CountTasks(seccionChecklist)`.

**Alternativa descartada.** *Pasar el documento entero a `CountTasks`*, como hacen hoy `dashboard.inspectIncrement` (`service.go:234`) y `tui.inspectIncrementForTUI` (`model.go:5941`) con `tasks.md`.

**Justificación.** `multirole.CountTasks` (`barrier.go:16-41`) cuenta **toda** línea que empiece por `- [ ]` o `- [x]`, sin ninguna noción de sección. En `tasks.md` eso es correcto porque el fichero entero es una lista de tareas. En el documento ODD **no lo es**: la plantilla canónica usa casillas también en «Criterios de aceptación», y nada impide que el usuario las use en «Comprobaciones aplicables» o «Evidencia de verificación». Pasar el documento completo produciría un porcentaje sistemáticamente falso, mostrado en la CLI, en la TUI y en el dashboard. La reutilización del contador sigue siendo real; lo que cambia es la entrada que se le da.

---

### D-05 — Tres estados de espejo, sondeo opcional, y **ningún** estado altera el código de salida

**Elección.**

```go
type MirrorState string

const (
    MirrorSynced      MirrorState = "sincronizado"
    MirrorDiverged    MirrorState = "divergente"
    MirrorUnavailable MirrorState = "no disponible"
)
```

El sondeo sólo se ejecuta con `--check-mirror` (CLI) o con una acción deliberada del usuario (TUI/Web UI). Nunca en el camino de refresco. `axiom odd status` devuelve **0** en los tres estados.

La comparación es: normalizar ambos lados (CRLF → LF, recorte de espacios finales por línea, eliminación de líneas en blanco terminales) y comparar SHA-256. El cuerpo candidato se extrae de la observación localizando el centinela `# ODD: <feature>` y tomando desde ahí hasta el final del contenido. Si el centinela no aparece, el resultado es `no disponible` con `Reason: "el espejo no contiene un cuerpo de documento reconocible"`, **no** `divergente`.

**Alternativas descartadas.**

1. *Comparar el contenido íntegro de la observación contra el fichero.* Siempre divergiría: la observación lleva el preámbulo propio de Engram. Un estado que siempre vale `divergente` no informa de nada.
2. *Comparar por marca temporal (mtime del fichero contra `updated` de la observación).* Falsos positivos ante cualquier `git checkout`, reescritura idempotente o clonado. Y falsos negativos si el agente guarda contenido distinto en el mismo minuto.
3. *Comparar por un `digest` declarado dentro del propio documento.* Autorreferencial: el digest de un documento que contiene su propio digest no es calculable sin una convención de exclusión frágil.
4. *Sondear el espejo en cada refresco de la TUI y en cada sondeo del dashboard.* Lanza un subproceso por refresco. Inaceptable en una TUI con *tick*, y explícitamente prohibido por §4.5 de la propuesta.
5. *Que `divergente` devuelva un código de salida distinto de 0.* Descartada como comportamiento **por defecto** (ver §9, pregunta abierta O-1). La propuesta garantiza que `no disponible` no altera el código de salida, pero no se pronuncia sobre `divergente`. Se elige el valor conservador: un informe no es un fallo, y un exit distinto de 0 rompería cualquier script que encadene `axiom odd status --check-mirror && ...`. Ausencia de centinela clasificada como `no disponible` en vez de `divergente` por la misma razón: no se afirma una divergencia que no se ha podido medir.

**Justificación.** El contrato del §4.5 pide un estado informado, barato por defecto, que nunca resuelva nada por su cuenta. La extracción por centinela es determinista, comprobable con tabla y no exige ninguna capacidad de escritura.

---

### D-06 — La marca de promoción es una cabecera **visible**, no metadatos ocultos

**Elección.** El documento vivo abre con un bloque de cita legible por humanos y parseable por prefijo de línea:

```markdown
# ODD: mi-feature

> **Estado:** promovido
> **Promovido a:** `openspec/changes/mi-cambio/`
> **Promovido el:** 2026-09-17
```

En estado activo, la cabecera contiene `> **Estado:** activo` y ninguna de las otras dos líneas.

**Alternativas descartadas.**

1. *Comentario HTML `<!-- odd:meta ... -->`.* Invisible al leer el fichero renderizado en GitHub o en un editor con vista previa. Un usuario podría seguir trabajando en un documento cerrado sin verlo, que es exactamente el riesgo R1.
2. *Front-matter YAML con `---`.* Exigiría una dependencia de parseo YAML o un parser propio, donde un escáner de líneas basta. Además rompe la continuidad visual con el resto del repositorio.
3. *Fichero lateral `odd/tasks/<feature>.state.json`.* Dos ficheros que pueden desincronizarse por un `git checkout` parcial, y una segunda cosa que mantener. El documento es la fuente de verdad; su estado pertenece al documento.

**Justificación.** El convenio de bloque de cita en cabecera ya es el del repositorio (`openspec/changes/*/proposal.md` abre exactamente así). Es visible, se parsea con `strings.HasPrefix` sobre líneas —el mismo estilo que `multirole.CountTasks`— y no añade dependencias.

---

### D-07 — Promoción en dos fases: **crear el cambio primero, marcar el documento después**

**Elección.** El orden es: validar → renderizar cuerpo → `Scaffolder.Scaffold(...)` → escribir la marca en el documento ODD. Nunca al revés.

Si la escritura de la marca falla tras una creación exitosa, `PromoteResult.Warning` transporta el aviso, el comando devuelve **1**, y stderr imprime la línea de remediación exacta:

```
Aviso: el cambio SDD se creó en openspec/changes/<nombre>/ pero no se pudo marcar
odd/tasks/<feature>.md como promovido (<causa>). Añade manualmente la línea
«> **Promovido a:** `openspec/changes/<nombre>/`» o elimina el directorio del cambio.
```

**Alternativas descartadas.**

1. *Marcar primero, crear después.* Si la creación falla —colisión con un archivado, permisos, disco lleno— queda un documento marcado como promovido que apunta a un cambio inexistente. El usuario pierde el carril ágil sin ganar el formal, y la guarda de idempotencia (D-08) le impide reintentar. Es el peor modo de fallo posible.
2. *Escritura atómica de ambos lados (fichero temporal + renombrado transaccional).* No existe transacción que cubra «crear un directorio con un fichero dentro» y «reescribir otro fichero en otra rama del árbol». Simularla con *rollback* exigiría borrar `openspec/changes/<nombre>/`, y §7.2 de la propuesta prohíbe que la CLI elimine datos de usuario ya creados.
3. *Silenciar el fallo de marcado y devolver 0.* Produce el escenario R1 (dos carriles vivos) sin que nadie se entere.

**Justificación.** Con el orden elegido, el único estado inconsistente posible es «cambio creado, documento sin marcar». Ese estado es **autocorrector en la dirección segura**: un segundo `axiom odd promote` de la misma *feature* choca contra la guarda de colisión de `service.go:776-778` («ya existe como cambio activo») y falla ruidosamente. Nunca se crean duplicados silenciosos.

---

### D-08 — Idempotencia por marca, con error centinela

**Elección.** `odd.Promote` carga el documento y, si `Status == StatusPromoted`, devuelve `fmt.Errorf("la feature %q ya fue promovida a %s: %w", feature, doc.PromotedTo, ErrAlreadyPromoted)` **antes** de invocar el puerto. El `Scaffolder` no llega a ejecutarse.

**Alternativa descartada.** *Apoyarse sólo en la guarda de colisión de `CreateIncrement`.* Cubre el caso habitual, pero no el de una promoción con `--name` distinto: `axiom odd promote mi-feature --name otro-nombre` crearía un segundo cambio SDD desde un documento ya cerrado, sin que ninguna guarda lo impidiese. La marca es la única defensa que cubre ese camino.

**Justificación.** Dos defensas independientes en dos capas distintas para el riesgo R1. La del dominio conoce la *feature*; la del andamiador conoce el nombre del cambio. Se necesitan ambas.

---

### D-09 — La promoción **nunca** deriva capacidades, y nombra el hueco

**Elección.** La sección `## Capacidades (Capabilities)` del cuerpo sembrado contiene exclusivamente:

```markdown
> **Pendiente para `sdd-spec`.** La promoción ODD no deriva capacidades: un documento
> ágil no las declara, y fabricarlas produciría rigor aparente. Esta sección debe
> redactarse en la fase de especificación antes de continuar.
```

Cualquier sección del documento ODD que esté vacía produce, en su destino, `_(sin contenido en el documento ODD de origen)_` — nunca texto inventado, nunca la sección omitida.

**Alternativas descartadas.**

1. *Derivar capacidades del checklist.* Una tarea no es una capacidad. Produciría nombres de capacidad plausibles y falsos, que `sdd-spec` tomaría por buenos y convertiría en deltas de especificación espurios.
2. *Omitir la sección.* La propuesta sembrada parecería estructuralmente completa. Un revisor no distingue «no aplica» de «nadie lo escribió todavía».
3. *Rellenar con la plantilla genérica vigente (`` `%s`: Funcionalidad principal introducida por el cambio ``, `service.go:827`).* Es exactamente el rigor aparente que R4 prohíbe, y además vendría firmado como si procediera del documento ODD.

**Justificación.** Riesgo R4 y §4.1.4 de la propuesta. Un hueco marcado es información; un hueco rellenado con plausibilidad es desinformación.

---

### D-10 — Validación de nombre **estrictamente más restrictiva** que la de incrementos, con denylist de nombres reservados de Windows

**Elección.** `odd.ValidateFeatureName` aplica, en este orden:

1. `^[a-z0-9]+(-[a-z0-9]+)*$` — el mismo patrón que `validIncrementNameRegex` (`service.go:762`).
2. Longitud ≤ 64 caracteres.
3. Denylist de nombres reservados del sistema de ficheros de Windows, en minúsculas: `con`, `prn`, `aux`, `nul`, `com1`…`com9`, `lpt1`…`lpt9`.
4. Comprobación defensiva de contención: tras `filepath.Join(root, "odd", "tasks", name+".md")`, `filepath.Rel(root, ruta)` no debe empezar por `..` ni ser absoluta.

**Invariante que esto garantiza:** *todo nombre válido para ODD es válido para `CreateIncrement`, pero no al revés.* La promoción, por tanto, nunca puede producir un nombre que el andamiador rechace por formato. Un test de propiedad sobre un corpus compartido lo verifica, ubicado en `internal/dashboard` (único paquete que ve `validIncrementNameRegex`, que es no exportada).

**Hallazgo colateral, deliberadamente no corregido.** `axiom change create con` es aceptado hoy por `validIncrementNameRegex` y crearía `openspec/changes/con/`, un nombre de dispositivo reservado en Windows —plataforma de desarrollo primaria de este repositorio— que impide el *checkout* del repositorio. Endurecer `validIncrementNameRegex` **cambiaría** el comportamiento observable de `axiom change create` y de `POST /api/increments`, que es precisamente lo que REQ-15.1 y el criterio de aborto §7.3 prohíben en este incremento. Se deja constancia como hallazgo para un incremento propio (§9, O-2) y se blinda sólo el camino ODD.

**Alternativas descartadas.**

1. *Reutilizar `validIncrementNameRegex` exportándola desde `dashboard`.* Obligaría a `internal/odd` a importar `internal/dashboard`, el ciclo de D-01.
2. *Confiar sólo en la regex.* Deja pasar `con`, `nul` y los 22 nombres reservados, y no acota la longitud de ruta en Windows.
3. *Endurecer también `validIncrementNameRegex`.* Regresión de REQ-15.1 y de la propiedad «ausencia de cuerpo sembrado ⇒ comportamiento idéntico». Fuera de alcance.

**Justificación.** El nombre de *feature* es el único dato controlado por el usuario que se convierte en un segmento de ruta del sistema de ficheros. Es la superficie de ataque de este incremento y se trata como tal (ver §7, filas T-6 y T-7).

---

### D-11 — Ambas superficies derivan del fichero, pero a través de **un único derivador**

**Elección.** `internal/dashboard` y `internal/tui` llaman ambas a `odd.Scan(root)`. No se copia el bucle de escaneo.

**Alternativa descartada.** *Replicar literalmente el patrón SDD*, es decir, que `dashboard.GetODDFeatures` y `tui.loadODDFeatures` contengan cada una su propia copia del `os.ReadDir` + parseo, como ocurre hoy con `inspectIncrement` (`service.go:200`) e `inspectIncrementForTUI` (`model.go:5915`), que son duplicación estructural exacta.

**Justificación.** El invariante que la propuesta pide preservar (§2.2, R6) es **la ausencia de acoplamiento entre superficies**: que la TUI no dependa del dashboard ni al revés, y que cada una re-derive del sistema de ficheros a su propia cadencia, sin consumir la proyección de la otra. Ese invariante se mantiene íntegro: ambas leen el disco de forma independiente. Lo que se evita es duplicar un parser de doce secciones y una máquina de estados —que, a diferencia de un `os.ReadDir` de seis líneas, sí divergiría. Nada en este diseño hace que la TUI dependa del dashboard. La duplicación SDD existente **no se toca**.

---

### D-12 — Las acciones de la TUI son **tipadas**, no índices calculados

**Elección.** `screens/odd_features.go` exporta, además de la lista de opciones, un resolvedor:

```go
type ODDAction int

const (
    ODDActionNone ODDAction = iota
    ODDActionSelectFeature
    ODDActionCreate
    ODDActionPromote
    ODDActionCheckMirror
    ODDActionGoToSDDLane
    ODDActionBack
)

// ODDFeaturesActionAt resuelve la acción correspondiente al cursor.
// Es la única fuente de verdad del mapeo cursor → acción.
func ODDFeaturesActionAt(features []ODDFeatureInfo, cursor int) (ODDAction, int)
```

`model.go` hace `switch action` sobre el valor devuelto. Nunca `if m.Cursor == len(m.ODDFeatures)+1`.

**Alternativa descartada.** *El patrón vigente de aritmética de índices* (`model.go:3153-3163`).

**Justificación.** Esa aritmética ya ha fallado en este mismo árbol: `screens/sdd_increments.go:33-34` declara las opciones «Avanzar fase del cambio seleccionado» y «Validar reporte de verificación», y el manejador de `ScreenSDDIncrements` (`model.go:3153-3163`) sólo contempla `Cursor < len(...)` y `Cursor == len(...)+1`, de modo que ambas opciones y «Crear nuevo incremento SDD» caen al `return m, nil` final: se pintan y no hacen nada. El fallo no es un descuido aislado, es el modo de fallo natural de tener la lista de opciones en un paquete y su semántica en otro, unidas por aritmética. Un resolvedor tipado en el mismo fichero que la lista hace estructuralmente imposible esa desincronización. **Este incremento no corrige el fallo preexistente de `ScreenSDDIncrements`** (fuera de alcance), pero no lo reproduce.

---

### D-13 — La conmutación de carril no modifica la lista de opciones de `ScreenSDDIncrements`

**Elección.**

- **ODD → SDD (TUI):** opción propia en `ScreenODDFeatures` («Ir al carril formal (SDD)»), más salto directo al pulsar Enter sobre una *feature* promovida, que fija `m.SDDActiveChange` y navega a `ScreenSDDIncrements`.
- **SDD → ODD (TUI):** una línea de cabecera **de sólo renderizado** añadida a `RenderSDDIncrements` — `Carril activo: SDD · esc → Gobernanza → «6» para el carril ágil (ODD)` — más la entrada 6 del menú de Gobernanza. No se añade ninguna opción a `SDDIncrementsOptions`.

**Alternativa descartada.** *Añadir «Ir al carril ágil (ODD)» a `SDDIncrementsOptions`.*

**Justificación.** `SDDIncrementsOptions` alimenta `screenOptionCount` (`model.go:4532-4533`) y, por tanto, los límites del cursor. Insertar una opción desplaza los índices que consume un manejador **que ya está desincronizado** (D-12). Convertiría un fallo latente y cosmético —tres opciones inertes— en un fallo activo de enrutamiento: pulsar «Volver a gobernanza» dejaría de volver. Se logra la paridad exigida por §10.3 con un cambio de renderizado de riesgo nulo.

---

### D-14 — Identificadores Go en inglés; **valores** serializados en castellano

**Elección.** `Status`, `StatusPromoted`, `MirrorDiverged` son identificadores; sus valores son `"promovido"` y `"divergente"`.

**Alternativa descartada.** *Valores en inglés (`"promoted"`) traducidos en cada capa de presentación.* Exigiría una tabla de traducción en la CLI, otra en la TUI y otra en `app.js`, con tres oportunidades de deriva, y la salida `--json` mostraría inglés en un producto cuyo `openspec/config.yaml` declara `language: "es"`.

**Justificación.** Es el convenio ya vigente en el repositorio: `internal/dashboard/service.go` usa identificadores ingleses (`CreateIncrement`, `humanizeName`) con mensajes y valores de cara al usuario en castellano. Los valores de `Status` y `MirrorState` se escriben literalmente en el documento vivo, que es un artefacto en castellano leído por humanos; traducirlos en presentación significaría que el fichero y la API dicen cosas distintas.

---

### D-15 — Los DTO de respuesta reutilizan los tipos del dominio; sólo las peticiones tienen DTO propio

**Elección.** `Service.GetODDFeatures() ([]odd.FeatureSummary, error)` y `Service.GetODDFeature(name string) (*odd.Document, error)` devuelven tipos de `internal/odd` con sus etiquetas JSON. En `types.go` sólo se añaden los DTO de **entrada**: `ODDCreateRequest`, `ODDPromoteRequest`, `ODDMirrorRequest`.

**Alternativa descartada.** *DTO espejo completos (`ODDFeatureSummaryDTO`, `ODDDocumentDTO`).* Duplica la forma en dos paquetes; cualquier campo añadido en el dominio y olvidado en el DTO desaparece silenciosamente de la API.

**Justificación.** El repositorio ya admite tipos de dominio en la frontera HTTP: `IncrementDetailDTO.BarrierReport` es un `*multirole.BarrierReport` directo (`types.go:56`). Se sigue ese precedente en vez de inventar una capa distinta para ODD.

---

### D-16 — Sin alias hifenados de nivel superior para `odd`

**Elección.** Se registra únicamente `case "odd"` en el `switch` de `cmd/axiom/main.go:179`. No se añaden `odd-create`, `odd-status` ni `odd-promote`.

**Alternativa descartada.** *Replicar el bloque `sdd-status`/`sdd-continue`/… (`main.go:353-366`).*

**Justificación.** Esos alias existen porque los activos distribuidos a los agentes los invocan como comandos de una sola palabra en un contrato publicado. ODD no tiene tal contrato: su consumidor es una persona en una terminal. Añadir siete puntos de entrada sin consumidor es superficie de API que después habría que sostener.

---

## 3. Flujo de Datos (Data Flow)

### 3.1 Lectura de estado (`status`, TUI, dashboard)

```
odd/tasks/*.md ──► odd.Scan(root) ──► []odd.Document ──► []odd.FeatureSummary
       │                  │
       │                  ├── odd.Parse: cabecera de estado + 12 secciones + checklist
       │                  └── multirole.CountTasks(sección checklist únicamente)   [D-04]
       │
       └── consumidores independientes, cada uno a su cadencia:
             ├── internal/cli/odd_status.go   ──► texto | --json
             ├── internal/dashboard (GET /api/odd) ──► sondeo del SPA
             └── internal/tui (loadODDFeatures) ──► ScreenODDFeatures
```

Ningún consumidor invoca a otro. El único subproceso posible en todo el camino es el sondeo del espejo, y sólo bajo petición explícita.

### 3.2 Sondeo del espejo (sólo bajo demanda)

```
--check-mirror ──► odd.CheckMirror(ctx, doc, exporter)
                         │
                         ├─ exporter = odd.DefaultExporter
                         │     exec.CommandContext(ctx, "engram", "export", <temp>)   [timeout 10 s]
                         │     cmd.Dir = root ; defer os.Remove(<temp>)
                         │
                         ├─ localizar observación con Topic == "odd/<feature>/tasks"
                         ├─ extraer cuerpo desde el centinela "# ODD: <feature>"
                         └─ sha256(normalizar(cuerpo)) == sha256(normalizar(doc.Raw))
                                   │
                                   ├── igual      ──► sincronizado
                                   ├── distinto   ──► divergente
                                   └── cualquier fallo, ausencia o centinela no hallado
                                                  ──► no disponible + Reason
```

Los tres desenlaces devuelven código de salida **0**. Go informa; no reconcilia [D-05].

### 3.3 Promoción

```
odd/tasks/<feature>.md
        │
        ├─ odd.Load ──► guarda de idempotencia (Status == promovido ⇒ ErrAlreadyPromoted)   [D-08]
        ├─ odd.ValidateFeatureName(nombre efectivo: --name o feature)                       [D-10]
        ├─ odd.RenderProposalBody(doc) ──► cuerpo Markdown completo, con § Capacidades
        │                                   marcada como pendiente                          [D-09]
        │
        ├─ --dry-run ⇒ escribir el cuerpo en stdout y TERMINAR (cero escrituras)
        │
        ├─ Scaffolder.Scaffold{Name, Intent, Type, ProposalBody}                            [D-01]
        │        └─► dashboard.Service.CreateIncrement
        │                 ├─ validIncrementNameRegex      (service.go:770)
        │                 ├─ colisión con activo          (service.go:776)
        │                 ├─ colisión con archivado       (service.go:782)
        │                 └─ os.WriteFile(proposal.md, ProposalBody)                        [D-02]
        │
        └─ odd.MarkPromoted(doc, changeName, today) ──► reescritura de la cabecera          [D-06]
                 └─ si falla: Warning + exit 1 + línea de remediación                       [D-07]
```

### 3.4 Diagrama de secuencia — carga y uso de la pantalla ODD en la TUI

```mermaid
sequenceDiagram
    actor U as Usuario
    participant M as tui.Model
    participant S as tui/screens
    participant O as internal/odd
    participant FS as Sistema de ficheros
    participant E as engram (subproceso)

    U->>M: Enter sobre «6. Carril Ágil ODD» en Gobernanza
    M->>M: loadODDFeatures()
    M->>O: Scan(".")
    O->>FS: ReadDir("odd/tasks")
    FS-->>O: ["mi-feature.md", "otra.md"]
    loop por cada documento
        O->>FS: ReadFile(ruta)
        O->>O: Parse: cabecera de estado + 12 secciones
        O->>O: CountTasks(solo sección checklist)
    end
    O-->>M: []odd.FeatureSummary
    M->>M: setScreen(ScreenODDFeatures)
    M-->>U: RenderODDFeatures(features, cursor, mensaje)

    Note over M,O: Ningún subproceso se ha lanzado todavía.

    U->>M: Enter sobre «Comprobar espejo Engram»
    M->>S: ODDFeaturesActionAt(features, cursor)
    S-->>M: ODDActionCheckMirror
    M-->>U: «Comprobando espejo…» (render inmediato)
    M->>O: CheckMirror(ctx, doc, DefaultExporter)
    O->>E: engram export <temp>  (timeout 10 s, cmd.Dir = root)
    alt binario ausente, exit≠0, JSON inválido o timeout
        E-->>O: fallo
        O-->>M: {State: "no disponible", Reason: "..."}
    else observación con topic odd/<feature>/tasks presente
        E-->>O: observations[]
        O->>O: extraer cuerpo por centinela + comparar SHA-256
        O-->>M: {State: "sincronizado" | "divergente"}
    end
    M-->>U: estado mostrado en GovernanceMessage; ninguna escritura, ningún bloqueo

    U->>M: Enter sobre una feature con estado «promovido»
    M->>S: ODDFeaturesActionAt(features, cursor)
    S-->>M: ODDActionSelectFeature
    M->>M: SDDActiveChange = feature.PromotedTo
    M->>M: loadSDDIncrements(); setScreen(ScreenSDDIncrements)
    M-->>U: carril formal, con el cambio promovido ya seleccionado
```

### 3.5 Diagrama de secuencia — promoción desde la CLI, incluido el fallo parcial

```mermaid
sequenceDiagram
    actor U as Usuario
    participant Main as cmd/axiom/main.go
    participant C as cli.RunODDPromote
    participant A as dashboardScaffolder
    participant O as odd.Promote
    participant D as dashboard.Service
    participant FS as Sistema de ficheros

    U->>Main: axiom odd promote mi-feature --name mi-cambio
    Main->>C: runODD(["promote", ...])
    C->>O: Promote(opts, scaffolder)

    O->>FS: Load("odd/tasks/mi-feature.md")
    alt documento ya promovido
        O-->>C: ErrAlreadyPromoted (envuelto con %w)
        C-->>U: «Error: la feature ... ya fue promovida a ...» · exit 1
    else documento activo
        O->>O: ValidateFeatureName("mi-cambio")
        O->>O: RenderProposalBody(doc)

        alt --dry-run
            O-->>C: PromoteResult{DryRun: true, Body: ...}
            C-->>U: cuerpo por stdout · exit 0 · CERO escrituras
        else ejecución real
            O->>A: Scaffold{Name, Intent, Type, ProposalBody}
            A->>D: CreateIncrement(CreateIncrementRequest{..., ProposalBody})
            D->>D: validIncrementNameRegex · colisión activo · colisión archivado
            alt colisión o nombre inválido
                D-->>A: error
                A-->>O: error
                O-->>C: error envuelto
                C-->>U: «Error: el incremento ... ya existe ...» · exit 1
                Note over FS: documento ODD intacto; reintento posible con otro --name
            else creación correcta
                D->>FS: MkdirAll + WriteFile(proposal.md, ProposalBody)
                D-->>A: {Name, Path}
                A-->>O: ScaffoldResult
                O->>FS: MarkPromoted: reescribir cabecera del documento ODD
                alt escritura de la marca correcta
                    O-->>C: PromoteResult{MarkWritten: true}
                    C-->>U: resumen + recordatorio de espejo pendiente · exit 0
                else escritura de la marca fallida
                    O-->>C: PromoteResult{MarkWritten: false, Warning: ...}
                    C-->>U: aviso + línea de remediación exacta · exit 1
                    Note over FS: único estado inconsistente posible.<br/>Un segundo promote choca con la guarda<br/>de colisión: nunca hay duplicado silencioso.
                end
            end
        end
    end
```

---

## 4. Cambios de Ficheros (File Changes)

### 4.1 Rebanada P1 — núcleo del documento vivo

| Fichero | Acción | Descripción |
|---|---|---|
| `internal/odd/document.go` | Crear | Tipos `Document`, `Task`, `SectionID`, `Status`, `FeatureSummary`, `StatusReport`. |
| `internal/odd/errors.go` | Crear | Errores centinela; todos envueltos con `%w` en los puntos de propagación. |
| `internal/odd/name.go` | Crear | `ValidateFeatureName`, denylist de nombres reservados, `DocumentPath` con comprobación de contención [D-10]. |
| `internal/odd/template.go` | Crear | Plantilla canónica de 12 secciones en castellano; `RenderNew(feature, today) string`, determinista. |
| `internal/odd/parse.go` | Crear | `Parse(raw) (*Document, error)`: cabecera de estado, troceado por `##`, checklist con IDs estables, progreso sobre la rebanada del checklist [D-04]. |
| `internal/odd/store.go` | Crear | `Create`, `Load`, `Scan`, `MarkPromoted`. Única capa con E/S de ficheros del paquete. |
| `internal/odd/mirror.go` | Crear | `MirrorState`, `MirrorReport`, `Observation`, puerto `Exporter`, `DefaultExporter` (`exec.CommandContext`, timeout 10 s), `CheckMirror` [D-05]. |
| `internal/odd/render.go` | Crear | `RenderStatusText(report) string` para la salida de texto de la CLI. |
| `internal/odd/*_test.go` | Crear | Tablas por unidad (§6). |
| `odd/tasks/.gitkeep` | Crear | Materializa el directorio versionado (decisión D1 cerrada: los documentos ODD se versionan). |

### 4.2 Rebanada P2 — CLI `create` y `status`

| Fichero | Acción | Descripción |
|---|---|---|
| `internal/odd/args.go` | Crear | `ParseCreateArgs`, `ParseStatusArgs`, `ParsePromoteArgs`: parseo puro, sin `io.Writer`, comprobable con tabla. Sigue el precedente de `sddstatus.ParseCommandArgs`. |
| `internal/cli/odd_create.go` | Crear | `RunODDCreate(args []string, stdout io.Writer) error`. Adaptador fino, al estilo de `sdd_status.go:13-40`. |
| `internal/cli/odd_status.go` | Crear | `RunODDStatus(args []string, stdout io.Writer) error`; ramas texto y `--json`; `--check-mirror` opcional. |
| `cmd/axiom/main.go` | Modificar | `case "odd"` en el `switch` (`:179`); `runODD(args, stdout, stderr) int` calcado de `runSDD` (`:1704-1750`); tres líneas nuevas en `printHelp()` (`:56-125`) y un ejemplo. |

### 4.3 Rebanada P3 — promoción

| Fichero | Acción | Descripción |
|---|---|---|
| `internal/dashboard/create_increment_characterization_test.go` | Crear | **Primero de la rebanada.** Bytes exactos de la plantilla vigente [D-03]. |
| `internal/dashboard/types.go` | Modificar | Campo `ProposalBody string \`json:"proposal_body,omitempty"\`` en `CreateIncrementRequest` (`:122-126`). |
| `internal/dashboard/service.go` | Modificar | Rama condicional de 3 líneas entre `:836` y `:838` [D-02]. Nada más. |
| `internal/odd/promote.go` | Crear | Puerto `Scaffolder`, `PromoteOptions`, `PromoteResult`, `RenderProposalBody`, `Promote` (secuencia de dos fases) [D-01, D-07, D-09]. |
| `internal/cli/odd_promote.go` | Crear | `RunODDPromote` + `dashboardScaffolder` (único punto del incremento donde `cli` importa `dashboard`). |
| `cmd/axiom/main.go` | Modificar | Subcomando `promote` en `runODD` y su línea de ayuda. |
| `internal/dashboard/name_parity_test.go` | Crear | Propiedad: todo nombre ODD-válido es incremento-válido [D-10]. |

### 4.4 Rebanada P4 — Web UI

| Fichero | Acción | Descripción |
|---|---|---|
| `internal/dashboard/odd_service.go` | Crear | `GetODDFeatures`, `GetODDFeature`, `CreateODDFeature`, `PromoteODDFeature`, `CheckODDMirror`. Fichero propio para no inflar `service.go`. |
| `internal/dashboard/types.go` | Modificar | DTO de **entrada** únicamente: `ODDCreateRequest`, `ODDPromoteRequest`, `ODDMirrorRequest` [D-15]. |
| `internal/dashboard/server.go` | Modificar | Cinco registros en `registerRoutes()` (`:40-72`) y sus manejadores, calcados de `handleIncrements` (`:251-275`). |
| `internal/dashboard/assets/index.html` | Modificar | Botón de navegación `data-tab="tab-odd"` tras `tab-increments` (`:40-42`) y `<section id="tab-odd">` con cabecera, filtros (Todos/Activos/Promovidos), botón «+ Nuevo documento ODD», botón «Comprobar espejo» y `#odd-container`. |
| `internal/dashboard/assets/app.js` | Modificar | `loadODD()`, `renderODDFeatures()`, `createODDFeature()`, `promoteODDFeature()`, `checkODDMirror()`, `focusIncrement(name)`. En `renderIncrements()` (`:704`), una insignia «← Origen ODD» construida desde el mapa cliente `promoted_to → feature`: **sin** cambios en el endpoint de incrementos. |
| `internal/dashboard/assets/style.css` | Modificar | `.lane-badge`, `.lane-badge--promoted`, `.mirror-state--*`. Reutiliza `.increments-grid` y las clases de tarjeta existentes. |

Rutas registradas (el `ServeMux` resuelve por patrón más largo, igual que con `/api/increments/continue` frente a `/api/increments/`):

| Método y ruta | Cuerpo | Efecto |
|---|---|---|
| `GET /api/odd` | — | `[]odd.FeatureSummary`. Sin subprocesos. Apto para sondeo. |
| `POST /api/odd` | `ODDCreateRequest` | Crea el documento vivo. `201`. |
| `GET /api/odd/{feature}` | — | `*odd.Document`. |
| `POST /api/odd/promote` | `ODDPromoteRequest{Feature, Name, DryRun}` | Promueve o previsualiza. |
| `POST /api/odd/check-mirror` | `ODDMirrorRequest{Feature}` | Sondeo deliberado. `POST` porque lanza un subproceso: nunca debe caer en un `GET` que un sondeo pueda repetir. |

### 4.5 Rebanada P5 — TUI

| Fichero | Acción | Descripción |
|---|---|---|
| `internal/tui/screens/odd_features.go` | Crear | `ODDFeatureInfo`, `ODDFeaturesOptions`, `ODDAction`, `ODDFeaturesActionAt`, `RenderODDFeatures` [D-12]. |
| `internal/tui/screens/governance.go` | Modificar | Entrada «6. Carril Ágil ODD (documentos vivos, promoción)» antes de «Volver al menú principal» (`:11-18`). |
| `internal/tui/screens/sdd_increments.go` | Modificar | **Sólo renderizado**: línea de cabecera con el carril activo y la ruta al carril ágil. La lista de opciones no cambia [D-13]. |
| `internal/tui/router.go` | Modificar | `ScreenODDFeatures: {Backward: ScreenGovernance}` (`:66-69`). |
| `internal/tui/model.go` | Modificar | Cinco puntos: constante `ScreenODDFeatures` (`:584`); `case` en `View()` (`:1662`); `case` en `screenOptionCount` (`:4532`); **entrada 6 y desplazamiento de «Volver» a 6→7** en el manejador de `ScreenGovernance` (`:3114-3134`); `case ScreenODDFeatures` en el manejador de Enter; `loadODDFeatures()` junto a `loadSDDIncrements()` (`:5882`). |

> **Lista de verificación de integración TUI** (los cinco puntos de `model.go` más `router.go` y `governance.go` deben moverse juntos; omitir cualquiera produce una pantalla muda o un cursor fuera de rango):
> - [ ] constante de pantalla · [ ] `View()` · [ ] `screenOptionCount` · [ ] ruta de retroceso · [ ] opción de Gobernanza · [ ] manejador de Gobernanza con «Volver» en 6 · [ ] manejador de la pantalla ODD · [ ] cargador.

### 4.6 Rebanada P6 — activos y documentación

| Fichero | Acción | Descripción |
|---|---|---|
| `internal/assets/…` (directrices ODD) | Crear/Modificar | Directrices, prompts y plantillas ODD en castellano bajo la Persona `axiom`. Renderizado determinista, sin vínculo evento → acción (R7, `organic-agent-trigger-rules`). |
| `docs/ROADMAP.md` | Modificar | Reescritura de INC-19 (`:278-289`), fila de catálogo (`:66`), alta de INC-20, contador de Fase 4 (`:8`). |
| `AGENTS.md`, `GEMINI.md` | Modificar (menor) | Referencia a los comandos ya existentes. |
| `.gitignore` | **Sin cambios** | Decisión D1 cerrada: los documentos ODD se versionan. La exclusión del presupuesto de 400 líneas es una regla de proceso para `sdd-tasks` y `sdd-apply`, no una regla de `.gitignore`. |

### 4.7 Ficheros prohibidos (criterio de aceptación por rebanada)

`internal/sddstatus/**`, `internal/cli/sdd_*.go`, `internal/agents/researchcapability/**`, `openspec/INDEX.md`, `openspec/config.yaml`. Ningún diff de ninguna rebanada puede contenerlos.

---

## 5. Interfaces y Contratos (Interfaces / Contracts)

### 5.1 Modelo del documento vivo

```go
package odd

// Status refleja el carril activo del documento vivo.
// Los valores se escriben literalmente en el fichero y se muestran al usuario. [D-14]
type Status string

const (
	StatusActive   Status = "activo"
	StatusPromoted Status = "promovido"
)

// SectionID identifica cada una de las doce secciones canónicas.
type SectionID string

const (
	SectionObjective       SectionID = "objetivo"
	SectionProblem         SectionID = "problema"
	SectionWhy             SectionID = "porque"
	SectionScope           SectionID = "alcance"
	SectionConstraints     SectionID = "restricciones"
	SectionAuthorizedScope SectionID = "alcance-autorizado"
	SectionChecklist       SectionID = "checklist"
	SectionAcceptance      SectionID = "criterios-de-aceptacion"
	SectionChecks          SectionID = "comprobaciones"
	SectionProgress        SectionID = "progreso"
	SectionEvidence        SectionID = "evidencia"
	SectionNextStep        SectionID = "siguiente-paso"
)

// CanonicalSections enumera las secciones en su orden canónico.
// La plantilla, el parser y el renderizador de promoción leen de aquí.
var CanonicalSections = []SectionID{ /* las doce, en orden */ }

// Task es una entrada del checklist con identificador estable.
type Task struct {
	ID   string `json:"id"`   // "T1", "T2", …; vacío si la línea no lo declara
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// Document modela odd/tasks/<feature>.md ya analizado.
type Document struct {
	Feature    string                     `json:"feature"`
	Path       string                     `json:"path"`        // relativa a la raíz del workspace
	Status     Status                     `json:"status"`
	PromotedTo string                     `json:"promoted_to,omitempty"`
	PromotedAt string                     `json:"promoted_at,omitempty"`
	Sections   map[SectionID]string       `json:"sections"`
	Tasks      []Task                     `json:"tasks"`
	Progress   multirole.RoleTaskProgress `json:"progress"`
	Warnings   []string                   `json:"warnings,omitempty"` // secciones ausentes, tareas sin ID
	Raw        string                     `json:"-"`                  // contenido íntegro: base de comparación del espejo
}
```

`Warnings` es el canal de degradación honesta: un documento editado a mano al que le falte una sección se analiza igual, con el hueco declarado. `Parse` sólo devuelve error cuando el documento carece de encabezado `# ODD: <feature>`, es decir, cuando no es un documento ODD.

### 5.2 Nombre y ruta

```go
// ValidateFeatureName aplica formato kebab-case, longitud máxima y denylist
// de nombres reservados del sistema de ficheros. Es estrictamente más
// restrictiva que validIncrementNameRegex. [D-10]
func ValidateFeatureName(name string) error

// DocumentPath devuelve la ruta absoluta del documento y verifica que quede
// contenida bajo root tras el Join (defensa en profundidad).
func DocumentPath(root, name string) (string, error)
```

### 5.3 Almacén (única capa con E/S)

```go
func Create(root, feature, today string) (*Document, error) // ErrFeatureExists si ya existe
func Load(root, feature string) (*Document, error)          // ErrFeatureNotFound
func Scan(root string) ([]FeatureSummary, error)            // directorio ausente ⇒ lista vacía, sin error
func MarkPromoted(root string, doc *Document, changeName, today string) error
```

`Scan` trata la ausencia de `odd/tasks/` como cero *features*, no como fallo: un workspace sin carril ágil es un estado normal, igual que `GetIncrements` ignora el error de `os.ReadDir` (`service.go:165`).

### 5.4 Espejo Engram (solo lectura, por firma)

```go
type MirrorState string

const (
	MirrorSynced      MirrorState = "sincronizado"
	MirrorDiverged    MirrorState = "divergente"
	MirrorUnavailable MirrorState = "no disponible"
)

type MirrorReport struct {
	State  MirrorState `json:"state"`
	Topic  string      `json:"topic"`
	Reason string      `json:"reason,omitempty"`
}

type Observation struct {
	Topic   string `json:"topic"`
	Content string `json:"content"`
}

// Exporter es el puerto de lectura del espejo. No existe ningún puerto de escritura,
// y el diseño prohíbe añadirlo: Go nunca escribe en Engram. [principio §4.1.2]
type Exporter func(ctx context.Context, root string) ([]Observation, error)

func DefaultExporter(ctx context.Context, root string) ([]Observation, error)

// CheckMirror no devuelve error: cualquier fallo se proyecta a MirrorUnavailable
// con su causa en Reason. Ningún estado altera el código de salida. [D-05]
func CheckMirror(ctx context.Context, doc *Document, export Exporter) MirrorReport

// MirrorTopic devuelve el topic canónico: "odd/<feature>/tasks".
func MirrorTopic(feature string) string
```

### 5.5 Promoción

```go
type ScaffoldRequest struct {
	Name         string
	Intent       string
	Type         string
	ProposalBody string
}

type ScaffoldResult struct {
	Name string
	Path string
}

// Scaffolder abstrae el andamiador de incrementos. Lo implementa
// internal/cli/odd_promote.go sobre dashboard.Service.CreateIncrement. [D-01]
type Scaffolder interface {
	Scaffold(ScaffoldRequest) (ScaffoldResult, error)
}

type PromoteOptions struct {
	Root       string
	Feature    string
	ChangeName string // --name; vacío ⇒ Feature verbatim
	DryRun     bool
	Today      string // inyectado para determinismo en tests
}

type PromoteResult struct {
	Feature      string `json:"feature"`
	ChangeName   string `json:"change_name"`
	ProposalPath string `json:"proposal_path,omitempty"`
	Body         string `json:"-"`
	DryRun       bool   `json:"dry_run"`
	MarkWritten  bool   `json:"mark_written"`
	MirrorNote   string `json:"mirror_note"` // siempre: el espejo queda PENDIENTE, lo actualiza el agente
	Warning      string `json:"warning,omitempty"`
}

func RenderProposalBody(doc *Document, changeName, today string) string
func Promote(opts PromoteOptions, sc Scaffolder) (*PromoteResult, error)
```

`MirrorNote` se rellena siempre, incluso en el camino feliz: la CLI nunca da por actualizado el espejo [principio §4.1.2].

### 5.6 Errores centinela y envoltorio

```go
var (
	ErrInvalidFeatureName = errors.New("nombre de feature no válido")
	ErrReservedName       = errors.New("nombre reservado por el sistema de ficheros")
	ErrPathEscape         = errors.New("la ruta resultante escapa de la raíz del workspace")
	ErrFeatureExists      = errors.New("el documento ODD ya existe")
	ErrFeatureNotFound    = errors.New("documento ODD no encontrado")
	ErrAlreadyPromoted    = errors.New("el documento ODD ya fue promovido")
	ErrMalformedDocument  = errors.New("documento ODD malformado")
)
```

Todo error saliente se envuelve con contexto y `%w`, conforme a la skill `axiom-idiomatic-error-wrapping`. Nunca `%v` ni `%s` para errores propagados:

```go
if err := os.WriteFile(path, []byte(content), 0644); err != nil {
    return fmt.Errorf("error escribiendo el documento ODD en %s: %w", path, err)
}
if !validFeatureNameRegex.MatchString(name) {
    return fmt.Errorf("el nombre %q debe estar en minúsculas kebab-case: %w", name, ErrInvalidFeatureName)
}
```

Las capas superiores deciden con `errors.Is`: `internal/cli` distingue `ErrAlreadyPromoted` (mensaje de idempotencia) de `ErrFeatureNotFound` (mensaje de ayuda con la lista de *features* disponibles). Sin `%w`, esa distinción exigiría comparar cadenas.

### 5.7 Contrato de la CLI

| Comando | Banderas | Salida | Código |
|---|---|---|---|
| `axiom odd create <nombre>` | `--cwd` | Ruta creada | 0 · 1 si nombre inválido, reservado o ya existe |
| `axiom odd status [feature]` | `--cwd`, `--json`, `--check-mirror` | Tabla o JSON | **0 en los tres estados de espejo** [D-05] · 1 si `--cwd` no existe o la *feature* nombrada no existe |
| `axiom odd promote <feature>` | `--cwd`, `--name`, `--intent`, `--type`, `--dry-run` | Resumen o cuerpo | 0 · 1 si ya promovida, nombre inválido, colisión, o marca no escrita [D-07] |

`--dry-run` no escribe absolutamente nada: ni la propuesta, ni el directorio del cambio, ni la marca.

### 5.8 Cuerpo sembrado — mapeo exacto

| Origen (documento ODD) | Destino (`proposal.md` sembrado) | Si el origen está vacío |
|---|---|---|
| Cabecera de procedencia | Bloque de cita: origen `odd/tasks/<feature>.md`, fecha, aviso de documento sembrado | n/a (siempre presente) |
| Objetivo | `## Propósito (Intent)`, párrafo inicial | `_(sin contenido en el documento ODD de origen)_` |
| Problema | `### Problema` | ídem |
| Porqué | `### Por qué ahora` | ídem |
| Alcance (entra) | `### Dentro de Alcance (In Scope)` | ídem |
| Alcance autorizado | `#### Alcance autorizado heredado` | ídem |
| Alcance (no entra) | `### Fuera de Alcance (Out of Scope)` | ídem |
| Restricciones | `#### Restricciones heredadas` | ídem |
| — | `## Capacidades (Capabilities)` → **marcador de pendiente para `sdd-spec`** [D-09] | — |
| Checklist (texto, en orden) | `## Enfoque (Approach)`, lista numerada | ídem |
| Criterios de aceptación | `## Criterios de Éxito` | ídem |
| Comprobaciones aplicables | `### Comprobaciones aplicables` | ídem |
| Checklist (con estado) + Progreso | `## Estado heredado de ODD`, tabla `Tarea \| Estado` + línea de progreso | tabla vacía con nota |
| Evidencia de verificación | `### Evidencia de verificación observada` | ídem |
| Siguiente paso | `### Siguiente paso declarado en ODD` | ídem |

El título se deriva del nombre del cambio con una función propia de `internal/odd`; `humanizeName` de `dashboard` no es exportada y no puede reutilizarse sin crear la arista prohibida [D-01]. La duplicación es de seis líneas y afecta sólo a un texto cosmético.

---

## 6. Estrategia de Pruebas (Testing Strategy)

`strict_tdd: true`. Todo test listado se escribe en RED, con fallo observado y registrado, **antes** de la producción que lo satisface. Convención obligatoria: tabla de structs anónimos + `t.Run`, sin `os.Exit` ni `panic`, `t.Fatalf` para precondiciones y `t.Errorf` para aserciones (skill `axiom-go-table-tests`).

| Capa | Qué se prueba | Cómo | Rebanada |
|---|---|---|---|
| Unit — nombre | Kebab-case válido/inválido, `..`, `/`, `\`, absoluto, vacío, >64, nombres reservados (`con`, `nul`, `com1`…`lpt9`), contención tras `Join` | Tabla sobre `ValidateFeatureName` y `DocumentPath` | P1 |
| Unit — plantilla | Las doce secciones presentes y en orden; dos invocaciones con la misma entrada producen bytes idénticos; salto de línea terminal | Tabla + comparación de cadena íntegra | P1 |
| Unit — parser | Cabecera activa/promovida; sección ausente ⇒ `Warnings`, no error; tarea sin ID ⇒ `Warnings`; documento sin `# ODD:` ⇒ `ErrMalformedDocument`; CRLF | Tabla con documentos literales | P1 |
| Unit — progreso | **Casillas en tres secciones distintas; sólo cuentan las del checklist** [D-04]; checklist vacío ⇒ 0/0 sin división por cero | Tabla con documento adversarial | P1 |
| Unit — almacén | `Create` duplicado ⇒ `ErrFeatureExists`; `Load` inexistente ⇒ `ErrFeatureNotFound`; `Scan` sin directorio ⇒ lista vacía sin error; `MarkPromoted` idempotente en forma | `t.TempDir()` | P1 |
| Unit — espejo | `sincronizado`; `divergente`; exportador que falla, que devuelve vacío, sin topic, con topic pero sin centinela ⇒ los cuatro a `no disponible` con `Reason` distinto; normalización CRLF y espacios finales | Tabla con `Exporter` falso | P1 |
| Unit — espejo real | `DefaultExporter` con `PATH` apuntando a un directorio vacío ⇒ error, y `CheckMirror` lo proyecta a `no disponible`; contexto ya cancelado ⇒ `no disponible`, sin bloqueo | `t.Setenv("PATH", t.TempDir())` + `context.WithCancel` | P1 |
| Unit — parseo de argumentos | Banderas conocidas y desconocidas, posicional ausente, `--json` con `--check-mirror`, `--cwd` relativo y absoluto | Tabla sobre `Parse*Args` | P2 |
| **Caracterización** | **Bytes exactos de la plantilla vigente de `CreateIncrement` sin `ProposalBody`** [D-03] | Tabla + comparación de cadena íntegra, **escrito antes de modificar la función** | **P3** |
| Unit — cuerpo sembrado | `## Capacidades` contiene el marcador y **ninguna** capacidad; secciones vacías ⇒ `_(sin contenido…)_`; cabecera de procedencia presente; tabla de estado heredado con el estado real de cada tarea | Tabla sobre `RenderProposalBody` | P3 |
| Unit — promoción | Ya promovida ⇒ `ErrAlreadyPromoted` **y el `Scaffolder` falso no se invoca**; `--dry-run` ⇒ cero invocaciones y cero escrituras; `Scaffolder` que falla ⇒ documento ODD intacto; `MarkPromoted` que falla ⇒ `Warning` poblado y `MarkWritten == false` | Tabla con `Scaffolder` falso instrumentado | P3 |
| Unit — paridad de nombres | Corpus compartido: todo nombre ODD-válido casa `validIncrementNameRegex` | Tabla en `internal/dashboard` (único paquete que ve la regex no exportada) | P3 |
| Unit — CLI | Códigos de salida por escenario; `--check-mirror` devuelve 0 en los tres estados; forma del `--json` | `bytes.Buffer` como `stdout`, al estilo de `internal/cli/command_output_test.go` | P2, P3 |
| Integración — HTTP | Los cinco endpoints vía `httptest.NewServer(srv.Router())`: `GET /api/odd` lista; `POST /api/odd` crea (201) y rechaza nombre inválido (400); `POST /api/odd/promote` con `dry_run`; `POST /api/odd/check-mirror`; método no permitido (405); **`GET /api/odd/{feature}` no colisiona con `/api/odd/promote`** | `net/http/httptest` (built-in, declarado en `config.yaml`) | P4 |
| Integración — regresión | `POST /api/increments` sin `proposal_body` produce la plantilla vigente; **con** `proposal_body` produce exactamente esos bytes | `httptest` + lectura del fichero | P3, P4 |
| TUI | Enter en Gobernanza índice 5 ⇒ `ScreenODDFeatures`; **Enter en índice 6 ⇒ `ScreenWelcome`** (protege el desplazamiento de «Volver»); `ODDFeaturesActionAt` devuelve la acción correcta para cada posición con 0, 1 y N *features*; Esc ⇒ `ScreenGovernance`; `screenOptionCount` coincide con `len(ODDFeaturesOptions(...))`; Enter sobre *feature* promovida ⇒ `ScreenSDDIncrements` con `SDDActiveChange` fijado | `Model.Update(tea.KeyMsg{...})` directo, sin `teatest`, conforme a `config.yaml` | P5 |
| Activos | Renderizado determinista de las directrices ODD; ausencia de vínculo evento → acción (R7) | Batería existente de `internal/assets/assets_test.go` | P6 |

**Verificación por rebanada.** Focalizada durante el trabajo (`go test ./internal/odd/... ./internal/cli/... ./internal/dashboard/... ./internal/tui/...`) y completa al cierre (`go test ./...`, `go vet ./...`, `gofmt -l` sobre los ficheros tocados), conforme a R8 y R9. El módulo `bench/` queda fuera y ninguna rebanada lo toca.

---

## 7. Matriz de Amenazas (Threat Matrix)

Aplicable: el diseño introduce enrutamiento de CLI, invocación de subproceso y construcción de rutas desde entrada del usuario.

### 7.1 Matriz canónica

| # | Frontera | Casos adversariales mínimos | Aplicabilidad | Respuesta de diseño | Tests RED planificados |
|---|---|---|---|---|---|
| T-1 | Rutas con apariencia de documentación | `requirements.txt`, `CMakeLists.txt`, Markdown/MDX ejecutable, `README.sh` | **N/A** — ODD no clasifica ningún fichero por contenido ni lo ejecuta. La extensión `.md` la añade Go (`name+".md"`); nunca procede de la entrada, y la regex prohíbe el punto. Los documentos son datos inertes de principio a fin. | — | Ninguno (fila N/A) |
| T-2 | Selección de repositorio | `git -C`, rutas relativas, rutas absolutas | **Aplicable** — `--cwd` determina la raíz, igual que en `axiom sdd` y `axiom change create`. | La raíz se resuelve una vez, al inicio del comando. Toda ruta se construye con `filepath.Join(root, …)` y se valida con `filepath.Rel` [D-10]. `--cwd` inexistente falla **antes** de cualquier `MkdirAll`. | `--cwd` relativo; `--cwd` absoluto; `--cwd` inexistente ⇒ exit 1 y cero directorios creados |
| T-3 | Estado del índice Git | preparado, `commit -a`, índice vacío | **N/A** — ODD no lee ni escribe el índice, no prepara, no confirma. Los documentos se versionan por decisión del usuario con su propio `git add`. | — | Ninguno (fila N/A) |
| T-4 | Estado de *push* | rama de seguimiento, primer *push*, *refspec* explícita | **N/A** — cero interacción con remotos. | — | Ninguno (fila N/A) |
| T-5 | Comandos de PR | `--head` explícito, prefijo de entorno, comandos compuestos | **N/A** — no se invoca `gh` ni ninguna automatización de PR. | — | Ninguno (fila N/A) |

### 7.2 Fronteras adicionales que este cambio sí introduce

| # | Frontera | Casos adversariales mínimos | Aplicabilidad | Respuesta de diseño | Tests RED planificados |
|---|---|---|---|---|---|
| T-6 | Invocación de subproceso (`engram export`) | binario ausente del `PATH`; salida distinta de cero; JSON malformado; proceso que no termina; fichero temporal filtrado | **Aplicable** | `exec.CommandContext` con *timeout* de 10 s. `argv` es un *slice* literal de tres elementos: **jamás** se compone una cadena de *shell* ni se interpola dato del usuario en los argumentos. Temporal por `os.CreateTemp` + `defer os.Remove`. Los cuatro fallos se proyectan a `no disponible` con `Reason`, exit 0 [D-05]. | Uno por caso: `PATH` vacío; exportador que devuelve error; JSON inválido; contexto cancelado. Más: el fichero temporal no existe al retornar. |
| T-7 | Segmento de ruta controlado por el usuario → escritura en disco | `..`, `../..`, `/etc/passwd`, `C:\Windows`, `a/b`, `a\b`, `con`, `nul`, `com1`, nombre de 300 caracteres, cadena vacía | **Aplicable** | Denegación por lista blanca (regex kebab-case) antes de cualquier `Join`, más límite de longitud, más denylist de nombres reservados de Windows, más comprobación de contención posterior al `Join` [D-10]. Cuatro capas, la primera de las cuales ya rechaza todos los casos listados. | Tabla única sobre `ValidateFeatureName` y `DocumentPath` con los once vectores, más verificación de que ningún fichero aparece fuera de `t.TempDir()` |
| T-8 | Enrutamiento de subcomandos de CLI | subcomando desconocido; sin subcomando; `--help`; `-h`; colisión con un `case` existente del `switch` | **Aplicable** | `runODD` calcado de `runSDD` (`main.go:1704-1750`): ayuda con salida 1 cuando faltan argumentos, 0 con `--help`, mensaje explícito y salida 1 ante subcomando desconocido. Verificado que no existe ningún `case "odd"` previo en el `switch` de `main.go:179`. Sin alias hifenados [D-16]. | Tabla de enrutamiento: `odd`, `odd --help`, `odd -h`, `odd inexistente`, los tres subcomandos válidos |
| T-9 | Contenido controlado por el usuario escrito verbatim en `proposal.md` | documento ODD con Markdown arbitrario, bloques de código, texto con forma de instrucción | **Aplicable, mitigación parcial y declarada** | El cuerpo sembrado es Markdown inerte y se escribe sin ejecutarse. La cabecera de procedencia declara siempre que el documento fue sembrado desde `odd/tasks/<feature>.md`, de modo que ningún revisor ni agente lo confunda con prosa redactada [D-09]. Axiom **no** sanea el contenido del documento del propio usuario en su propio repositorio: hacerlo destruiría contenido legítimo sin elevar ninguna frontera de confianza real, porque el usuario ya puede escribir directamente en `openspec/changes/`. | El cuerpo sembrado conserva bytes idénticos a la entrada; la cabecera de procedencia está siempre presente, también cuando el documento origen la contradice |

Todas las filas marcadas `Aplicable` se trasladan **sin modificación** a `tasks.md` como tests RED previos a la producción correspondiente. Las filas `N/A` no generan tarea alguna.

---

## 8. Migración y Despliegue (Migration / Rollout)

**No se requiere migración de datos.** No hay esquema persistido, no hay formato que convertir, y los 18 incrementos SDD archivados no se tocan. El espejo Engram es aditivo y versionado por `topic_key`.

**Despliegue por rebanadas apiladas contra `main`** (`stacked-to-main`, §4.6 de la propuesta):

```
main
 └─ P1 núcleo ──► P2 CLI create|status ──► P3 promoción ──┬─► P4 Web UI ──┐
                                                          └─► P5 TUI ─────┴─► P6 activos y docs
```

| Rebanada | Estado del árbol al cerrar | Frontera de reversión |
|---|---|---|
| P1 | Compila y verde. Paquete nuevo sin consumidores: el binario no cambia de comportamiento. | Aditivo puro. Revertir **después** de P2. |
| P2 | `axiom odd create\|status` operativos. | Elimina `internal/cli/odd_*.go`, el `case "odd"`, `runODD` y la ayuda. Revertir después de P3. |
| P3 | `axiom odd promote` operativo. **Puerta de control: la caracterización de D-03 debe seguir verde.** | Revierte `CreateIncrement` a su firma previa. Los cambios SDD ya promovidos **se conservan**: son datos. Revertir antes de P2. |
| P4 | Dashboard con carril ODD. | Elimina rutas y superficie SPA. Independiente de P5. |
| P5 | TUI con carril ODD. | Elimina la pantalla y su entrada de menú. Independiente de P4. |
| P6 | Activos y documentación al día. | Trivial. |

**Sin banderas de funcionalidad.** El cambio es aditivo: un workspace sin `odd/tasks/` ve una lista vacía en las tres superficies y no experimenta ninguna diferencia. Una bandera añadiría una ruta de código que nadie desactivaría nunca.

**Compatibilidad hacia atrás.** El único contrato existente que se toca es `CreateIncrementRequest`, y se toca añadiendo un campo opcional con `omitempty`: los clientes que no lo envían obtienen el comportamiento vigente byte a byte, protegido por D-03. Ningún endpoint, comando o pantalla existente cambia de forma.

**Criterios de aborto** (§7.3 de la propuesta, sin cambios): regresión de la plantilla vigente; pérdida de fidelidad de fase en `axiom ui` o `axiom tui`; cualquier fichero bajo `internal/sddstatus/` o `internal/cli/sdd_*.go` en un diff; cualquier escritura de la CLI hacia Engram.

---

## 9. Preguntas Abiertas (Open Questions)

- [ ] **O-1 — Código de salida de `axiom odd status --check-mirror` ante `divergente`.** Este diseño lo fija en **0** (un informe no es un fallo, y un valor distinto rompería `axiom odd status --check-mirror && …`). La propuesta garantiza explícitamente ese comportamiento para `no disponible`, pero no se pronuncia sobre `divergente`. Es un contrato de CLI del que podrían depender consumidores futuros. **Severidad baja; no bloquea ninguna rebanada** — la elección conservadora siempre puede ampliarse después con una bandera opcional sin romper a nadie, mientras que lo contrario sí rompería. Elevado al orquestador por si prefiere fijarlo en la especificación.

- [ ] **O-2 — Nombres de dispositivo reservados de Windows aceptados por `validIncrementNameRegex`.** Hallazgo verificado durante el diseño: `axiom change create con` crea hoy `openspec/changes/con/`, un nombre que impide el *checkout* del repositorio en Windows, la plataforma primaria de desarrollo. **Deliberadamente NO se corrige en este incremento**, porque endurecer esa regex cambiaría el comportamiento observable de `axiom change create` y de `POST /api/increments`, que es exactamente lo que REQ-15.1 y el criterio de aborto §7.3 prohíben aquí. El camino ODD sí queda blindado [D-10]. Registrado como candidato a incremento propio; no pertenece a INC-20, cuyo alcance es la retirada del contrato del motor SDD.

- [ ] **O-3 — Dónde se materializa la exclusión de los documentos ODD del presupuesto de 400 líneas.** La decisión D1 está cerrada (se versionan en Git, excluidos del recuento). Esa exclusión es una regla de proceso que consumen `sdd-tasks` y `sdd-apply`, no una regla expresable en `.gitignore` ni en código. Este diseño no la materializa en ningún fichero y la declara como pendiente de recogerse en el pronóstico de `sdd-tasks`.

**Ninguna de las tres bloquea el diseño ni el inicio de P1.**

---

## 10. Trazabilidad

| Elemento de la propuesta | Decisiones que lo cubren |
|---|---|
| §2.1.1 Documento vivo de 12 secciones + espejo | D-04, D-06, D-14; interfaces §5.1, §5.4 |
| §2.1.2 Superficie CLI | D-16; interfaces §5.7; matriz T-8 |
| §2.1.3 Promoción reutilizando `CreateIncrement` | D-01, D-02, D-07, D-08, D-09; §5.5, §5.8 |
| §2.1.4 Integración reactiva en ambas interfaces | D-11, D-12, D-13, D-15; §4.4, §4.5; diagrama §3.4 |
| §4.1.2 El fichero manda, Engram es espejo | D-05; el puerto `Exporter` carece de método de escritura, por firma |
| §4.5 Política de divergencia | D-05; §3.2; matriz T-6 |
| R1 Dos carriles vivos | D-06, D-08, D-13 |
| R2 Divergencia fichero/espejo | D-05 |
| R3 Regresión de REQ-15.1 | D-02, **D-03** |
| R4 Fabricación de rigor | D-09 |
| R5 Colisión o invalidez de nombre | D-10; guardas de `service.go:770-788` intactas |
| R6 Duplicación estructural | D-11 (se contiene sin tocar la duplicación SDD existente) |
| R7 Activos que incumplen `organic-agent-trigger-rules` | §6, fila «Activos» |
| R8 Ciclo de verificación lento | §6, verificación por rebanada |
| R9 Ruido de `gofmt` | §8; sólo se normalizan los ficheros tocados |
| R10 Arrastre de alcance | §4.7, lista de ficheros prohibidos |

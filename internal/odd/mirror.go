package odd

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// mirrorExportTimeout acota la duración del subproceso de exportación del
// espejo. El sondeo nunca espera sin límite: un binario colgado no debe
// bloquear el carril ágil [D-05, frontera T-6].
const mirrorExportTimeout = 10 * time.Second

// MirrorState enumera los tres desenlaces posibles de comparar un documento
// vivo contra su espejo de recuperación en Engram. Ninguno de los tres
// altera el código de salida de quien invoca la comparación: el lado Go
// informa, nunca reconcilia [D-05, REQ-19.4].
type MirrorState string

const (
	// MirrorSynced indica que, tras normalizar ambos lados, el espejo
	// coincide con el contenido del fichero.
	MirrorSynced MirrorState = "sincronizado"
	// MirrorDiverged indica que el espejo existe y se pudo comparar, pero su
	// contenido normalizado difiere del fichero. El sistema únicamente
	// informa: ni el fichero ni el espejo se modifican para reconciliar.
	MirrorDiverged MirrorState = "divergente"
	// MirrorUnavailable indica que no fue posible obtener del espejo un
	// cuerpo comparable: el exportador falló, no devolvió observaciones,
	// ninguna coincide con el topic esperado, o la que coincide no contiene
	// el centinela de cabecera reconocible. Reason transporta la causa
	// exacta de cuál de esos caminos ocurrió.
	MirrorUnavailable MirrorState = "no disponible"
)

// MirrorReport es el resultado informativo de comparar un documento vivo
// contra su espejo de recuperación en Engram.
type MirrorReport struct {
	State  MirrorState `json:"state"`
	Topic  string      `json:"topic"`
	Reason string      `json:"reason,omitempty"`
}

// Observation modela una observación de Engram ya traducida al vocabulario
// de este paquete: únicamente el topic y el contenido, lo único que
// CheckMirror necesita para localizar y comparar el espejo.
//
// El transporte real de "engram export" no lleva un campo "topic": el
// envoltorio JSON es {"observations": [...]}  y cada observación trae
// title/content/project/scope (sin topic), con el topic codificado dentro
// de title — el mismo convenio que ya usa la implementación probada en
// producción para el mismo subproceso (internal/sddstatus.engramObservation
// y exportEngramObservations, status.go:269-274 y 1077-1104). DefaultExporter
// (vía decodeEngramExportPayload) es quien puebla Topic a partir de Title al
// deserializar; un Exporter falso de test puede seguir construyendo
// Observation{Topic: ..., Content: ...} directamente, sin pasar por ese
// mapeo.
type Observation struct {
	Topic   string `json:"topic"`
	Content string `json:"content"`
}

// Exporter es el puerto de lectura del espejo de recuperación en Engram. No
// existe ningún puerto de escritura, y este paquete no lo añade: el lado Go
// nunca escribe en Engram, sólo informa de lo que encuentra en él
// [principio §4.1.2 del diseño].
type Exporter func(ctx context.Context, root string) ([]Observation, error)

// MirrorTopic devuelve el topic canónico bajo el que se busca el espejo de
// recuperación de feature en Engram (REQ-19.4).
func MirrorTopic(feature string) string {
	return fmt.Sprintf("odd/%s/tasks", feature)
}

// CheckMirror compara el documento vivo doc contra su espejo de
// recuperación en Engram, obtenido a través del puerto export. No devuelve
// error: cualquier fallo —del exportador, de la búsqueda por topic o de la
// extracción por centinela— se proyecta a MirrorUnavailable con su causa en
// Reason [D-05].
//
// root se reenvía tal cual a export: es el mismo valor que el resto del
// paquete (Create, Load, Scan, MarkPromoted) recibe para saber contra qué
// workspace operar. El contrato de interfaces del diseño (§5.4) no lo lista
// como parámetro explícito de CheckMirror, pero el propio puerto Exporter
// exige un root que en ningún otro punto de la firma está disponible; se
// añade aquí de forma aditiva, sin alterar MirrorState, MirrorReport,
// Observation ni Exporter.
func CheckMirror(ctx context.Context, root string, doc *Document, export Exporter) MirrorReport {
	topic := MirrorTopic(doc.Feature)

	observations, err := export(ctx, root)
	if err != nil {
		return MirrorReport{
			State:  MirrorUnavailable,
			Topic:  topic,
			Reason: fmt.Sprintf("no se pudo exportar el espejo Engram: %v", err),
		}
	}

	candidate, err := mirrorCandidateBody(observations, topic, headerPrefix+doc.Feature)
	if err != nil {
		return MirrorReport{State: MirrorUnavailable, Topic: topic, Reason: err.Error()}
	}

	if mirrorBodiesMatch(candidate, doc.Raw) {
		return MirrorReport{State: MirrorSynced, Topic: topic}
	}
	return MirrorReport{State: MirrorDiverged, Topic: topic}
}

// mirrorCandidateBody localiza, entre observations, la que coincide con
// topic y extrae su cuerpo candidato a partir de sentinel (la cabecera "#
// ODD: <feature>"), tomando desde ahí hasta el final del contenido. Cubre
// las tres formas en que el espejo puede no proyectar un cuerpo comparable:
// lista vacía, ninguna observación con ese topic, y topic coincidente sin
// centinela reconocible [D-05, frontera T-6].
//
// Limitación conocida y aceptada en esta rebanada: la búsqueda por topic no
// discrimina por proyecto (Project no forma parte de Observation; ver su
// GoDoc). Si dos proyectos distintos comparten el mismo topic
// "odd/<feature>/tasks" —porque coincide el nombre de feature—, se compara
// contra la primera observación que aparezca en observations, en el orden
// que la devuelva el exportador, sin ningún criterio de desempate
// adicional. Cubierto explícitamente por
// TestCheckMirror_LimitacionConocida_ColisionEntreProyectos; a confirmar o
// endurecer en la rebanada de la CLI (Fase 3) si resulta observable en un
// workspace real.
func mirrorCandidateBody(observations []Observation, topic, sentinel string) (string, error) {
	if len(observations) == 0 {
		return "", errors.New("el espejo no devolvió ninguna observación")
	}

	var content string
	found := false
	for _, obs := range observations {
		if obs.Topic == topic {
			content = obs.Content
			found = true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("no se encontró ninguna observación del espejo con el topic %q", topic)
	}

	idx := strings.Index(content, sentinel)
	if idx == -1 {
		return "", errors.New("el espejo no contiene un cuerpo de documento reconocible")
	}

	return content[idx:], nil
}

// mirrorBodiesMatch compara a y b tras normalizarlos independientemente,
// mediante SHA-256. La comparación por resumen evita retener en memoria una
// diferencia carácter a carácter que aquí no se necesita: sólo importa si
// coinciden o no [D-05].
func mirrorBodiesMatch(a, b string) bool {
	return sha256.Sum256([]byte(normalizeMirrorBody(a))) == sha256.Sum256([]byte(normalizeMirrorBody(b)))
}

// normalizeMirrorBody normaliza un cuerpo de documento antes de compararlo:
// unifica los saltos de línea CRLF a LF, recorta el espacio en blanco final
// de cada línea, y elimina las líneas en blanco terminales. La
// normalización absorbe diferencias de transporte irrelevantes —el editor
// del usuario, el agente que escribe en Engram— sin ocultar una divergencia
// real de contenido [D-05].
func normalizeMirrorBody(body string) string {
	unified := strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.Split(unified, "\n")

	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	return strings.Join(lines, "\n")
}

// engramExportPayload es el envoltorio JSON real que "engram export" escribe
// en el fichero temporal: un objeto con la clave "observations", nunca un
// array plano de observaciones. Forma verificada contra la implementación
// ya probada en producción para el mismo subproceso
// (internal/sddstatus.exportEngramObservations, status.go:1097-1099).
type engramExportPayload struct {
	Observations []engramExportObservation `json:"observations"`
}

// engramExportObservation modela una observación tal como la transporta
// "engram export". No existe ningún campo "topic" en el transporte real: el
// topic viaja codificado dentro de Title, con el mismo convenio que ya usa
// internal/sddstatus (engramTitlePattern, status.go:1178): para ODD,
// "odd/<feature>/tasks". Forma verificada contra
// internal/sddstatus.engramObservation (status.go:269-274).
type engramExportObservation struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Project string `json:"project"`
	Scope   string `json:"scope"`
}

// decodeEngramExportPayload interpreta el JSON que "engram export" escribe
// en su fichero de salida y lo traduce al vocabulario de este paquete:
// puebla Observation.Topic desde Title (recortado de espacio en blanco) y
// descarta las observaciones de ámbito "personal" — el mismo filtro de
// ámbito que aplica engramObservationMatchesProject (status.go:1241-1243).
//
// Limitación conocida y aceptada en esta rebanada: no se filtra por
// Project. internal/odd no puede importar internal/sddstatus (prohibido en
// este incremento) y duplicar aquí inferEngramProject (status.go:1106 y
// siguientes: variable de entorno, git config) sería exceso de alcance para
// esta rebanada. La desambiguación se apoya únicamente en la especificidad
// del topic más el centinela de cabecera que aplica mirrorCandidateBody; su
// GoDoc documenta el caso de colisión entre proyectos que esto deja
// abierto, cubierto por
// TestCheckMirror_LimitacionConocida_ColisionEntreProyectos.
func decodeEngramExportPayload(data []byte) ([]Observation, error) {
	var payload engramExportPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("la exportación del espejo Engram devolvió JSON malformado: %w", err)
	}

	observations := make([]Observation, 0, len(payload.Observations))
	for _, wire := range payload.Observations {
		if strings.TrimSpace(wire.Scope) == "personal" {
			continue
		}
		observations = append(observations, Observation{
			Topic:   strings.TrimSpace(wire.Title),
			Content: wire.Content,
		})
	}

	return observations, nil
}

// DefaultExporter implementa Exporter invocando "engram export <temporal>"
// como subproceso, con un timeout de 10 s. argv es un slice literal de tres
// elementos: nunca se compone una cadena de shell ni se interpola dato del
// usuario en los argumentos [frontera T-6]. El fichero temporal se crea con
// os.CreateTemp y se elimina de forma incondicional al retornar —con
// independencia de que la exportación tenga éxito, falle, o el contexto ya
// esté cancelado— porque el defer de eliminación se registra antes de
// cualquier otro punto de retorno posible.
func DefaultExporter(ctx context.Context, root string) ([]Observation, error) {
	tmp, err := os.CreateTemp("", "odd-mirror-export-*.json")
	if err != nil {
		return nil, fmt.Errorf("no se pudo crear el fichero temporal de exportación del espejo: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if closeErr := tmp.Close(); closeErr != nil {
		return nil, fmt.Errorf("no se pudo cerrar el fichero temporal de exportación del espejo: %w", closeErr)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, mirrorExportTimeout)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, "engram", "export", tmpPath)
	cmd.Dir = root

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("no se pudo ejecutar la exportación del espejo Engram: %w", err)
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer el fichero temporal de exportación del espejo: %w", err)
	}

	return decodeEngramExportPayload(data)
}

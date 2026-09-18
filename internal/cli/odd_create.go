package cli

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/odd"
)

// RunODDCreate es el punto de entrada de la CLI para "axiom odd create
// <nombre>" (REQ-19.5). Es un adaptador fino sobre odd.Create, al estilo de
// internal/cli/sdd_status.go:13-40 (read-only): interpreta los argumentos,
// resuelve la raíz de trabajo, delega en el dominio y traduce el resultado
// a la salida estándar.
func RunODDCreate(args []string, stdout io.Writer) error {
	parsed, err := odd.ParseCreateArgs(args)
	if err != nil {
		return err
	}

	if err := requireODDWorkDir(parsed.CWD); err != nil {
		return err
	}

	doc, err := odd.Create(parsed.CWD, parsed.Name, time.Now().Format("2006-01-02"))
	if err != nil {
		return fmt.Errorf("no se pudo crear el documento vivo de la feature %q: %w", parsed.Name, err)
	}

	_, err = fmt.Fprintf(stdout, "Documento vivo creado en %s\n", doc.Path)
	return err
}

// requireODDWorkDir comprueba que cwd existe y es un directorio antes de que
// cualquier subcomando de "axiom odd" delegue en el dominio. La comparten
// RunODDCreate y RunODDStatus (internal/cli/odd_status.go) por dos motivos
// distintos según el subcomando, ambos cubiertos por esta única
// comprobación: odd.Create crea con os.MkdirAll cualquier directorio
// intermedio ausente —así que sin esta comprobación un --cwd inexistente
// terminaría creándose en vez de rechazarse [frontera T-2 de la matriz de
// amenazas del diseño]—, y odd.Scan trata la ausencia de odd/tasks/ bajo un
// --cwd que sí existe como "cero features sin error" [REQ-19.6]: sin esta
// comprobación previa, un --cwd inexistente para "status" se confundiría
// silenciosamente con ese mismo estado vacío legítimo.
func requireODDWorkDir(cwd string) error {
	info, statErr := os.Stat(cwd)
	if statErr != nil {
		return fmt.Errorf("no se pudo acceder al directorio de trabajo %q: %w", cwd, statErr)
	}
	if !info.IsDir() {
		return fmt.Errorf("la ruta de trabajo %q no es un directorio", cwd) // refusal:by-design operator-knowledge: solo el operador puede indicar una ruta de trabajo valida; ningun comando de axiom puede convertir un fichero en directorio
	}
	return nil
}

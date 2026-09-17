package cli

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/odd"
)

// RunODDPromote es el punto de entrada de la CLI para "axiom odd promote
// <feature>" (REQ-19.8, REQ-19.9, REQ-19.10, REQ-19.12). Es un adaptador
// fino sobre odd.Promote: interpreta los argumentos, resuelve la raíz de
// trabajo, delega en el dominio a través del Scaffolder que newScaffolder
// construye para esa raíz, y traduce el resultado a la salida estándar y
// al código de salida del proceso vía el error devuelto.
//
// A diferencia de RunODDCreate y RunODDStatus, que resuelven internamente
// su única dependencia externa, RunODDPromote recibe newScaffolder como
// parámetro explícito en vez de construir un dashboardScaffolder interno
// [desviación deliberada de §5.5/§5.8 del diseño, documentada en el informe
// de esta fase]: internal/dashboard/service.go:19 ya importa internal/cli
// (para RunSDDContinue, RunSDDVerifyValidate, RunDoctor y RunRestore), así
// que si este paquete importara internal/dashboard para construir el
// adaptador aquí mismo, el grafo cerraría el ciclo cli → dashboard → cli
// -- con independencia total de internal/odd, que sigue siendo una hoja
// del árbol de dependencias sin este cambio. cmd/axiom/main.go, al que no
// importa ningún otro paquete, es el único punto del árbol de
// dependencias actual que puede construir el adaptador sobre
// dashboard.Service.CreateIncrement y pasarlo aquí sin ciclar.
//
// runODD (cmd/axiom/main.go) imprime "Error: %v" y devuelve el código de
// salida 1 ante cualquier error no nulo devuelto por esta función; por eso
// el fallo de marca de D-07 -- el único camino en el que odd.Promote no
// devuelve un error Go, sino un PromoteResult.Warning poblado -- se
// traduce aquí a un error para que ese único estado inconsistente posible
// también finalice con exit 1, tal como exige el contrato de CLI del
// diseño (§5.7) y su diagrama de secuencia (§3.5).
func RunODDPromote(args []string, stdout io.Writer, newScaffolder func(root string) odd.Scaffolder) error {
	parsed, err := odd.ParsePromoteArgs(args)
	if err != nil {
		return err
	}

	if err := requireODDWorkDir(parsed.CWD); err != nil {
		return err
	}

	result, err := odd.Promote(odd.PromoteOptions{
		Root:       parsed.CWD,
		Feature:    parsed.Feature,
		ChangeName: parsed.Name,
		Intent:     parsed.Intent,
		Type:       parsed.Type,
		DryRun:     parsed.DryRun,
		Today:      time.Now().Format("2006-01-02"),
	}, newScaffolder(parsed.CWD))
	if err != nil {
		return fmt.Errorf("no se pudo promover la feature %q a un cambio SDD: %w", parsed.Feature, err)
	}

	if result.DryRun {
		_, ferr := fmt.Fprint(stdout, result.Body)
		return ferr
	}

	// Único estado inconsistente posible de odd.Promote [D-07]: el cambio SDD
	// ya se creó, pero la marca de promoción no se pudo escribir en el
	// documento ODD de origen. result.Warning ya contiene la línea de
	// remediación exacta (internal/odd/promote.go: formatMarkFailureWarning);
	// se propaga verbatim, sin envolver, para que el operador la lea
	// literalmente.
	if result.Warning != "" {
		return errors.New(result.Warning)
	}

	if _, ferr := fmt.Fprintf(stdout, "Feature %q promovida al cambio SDD %q en %s\n", parsed.Feature, result.ChangeName, result.ProposalPath); ferr != nil {
		return ferr
	}
	_, ferr := fmt.Fprintln(stdout, result.MirrorNote)
	return ferr
}

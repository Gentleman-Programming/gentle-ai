package odd

import (
	"fmt"
	"strings"
)

// CreateArgs son los argumentos ya interpretados de "axiom odd create
// <nombre>" (REQ-19.5).
type CreateArgs struct {
	Name string
	CWD  string
}

// StatusArgs son los argumentos ya interpretados de "axiom odd status
// [feature]" (REQ-19.6, REQ-19.7). Feature queda vacío cuando el usuario no
// selecciona una feature concreta: el comando informa entonces de todos los
// documentos vivos existentes bajo odd/tasks/.
type StatusArgs struct {
	Feature     string
	CWD         string
	JSON        bool
	CheckMirror bool
}

// PromoteArgs son los argumentos ya interpretados de "axiom odd promote
// <feature>" (REQ-19.8). Su consumo desde la CLI (internal/cli/odd_promote.go)
// llega en una fase posterior; el parseo se cubre en esta rebanada porque
// comparte convención con ParseCreateArgs y ParseStatusArgs.
type PromoteArgs struct {
	Feature string
	CWD     string
	Name    string
	Intent  string
	Type    string
	DryRun  bool
}

// ParseCreateArgs interpreta los argumentos de "axiom odd create <nombre>".
// Es parseo puro: no accede al sistema de ficheros ni escribe en ningún
// io.Writer, siguiendo el precedente de sddstatus.ParseCommandArgs
// (`internal/sddstatus/status.go:278-321`, read-only). CWD queda en "." si
// el usuario no pasa --cwd: resolverlo frente al sistema de ficheros es
// responsabilidad del adaptador de CLI, no de este parseo.
func ParseCreateArgs(args []string) (CreateArgs, error) {
	const cmd = "axiom odd create"
	parsed := CreateArgs{CWD: "."}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--cwd":
			value, err := nextFlagValue(args, i, cmd, arg)
			if err != nil {
				return CreateArgs{}, err
			}
			parsed.CWD = value
			i++
		default:
			if strings.HasPrefix(arg, "-") {
				return CreateArgs{}, fmt.Errorf("%s: bandera desconocida %q", cmd, arg)
			}
			if parsed.Name != "" {
				return CreateArgs{}, fmt.Errorf("%s: argumento inesperado %q", cmd, arg)
			}
			parsed.Name = arg
		}
	}

	if parsed.Name == "" {
		return CreateArgs{}, fmt.Errorf("%s: falta el nombre de la feature como argumento posicional", cmd)
	}

	return parsed, nil
}

// ParseStatusArgs interpreta los argumentos de "axiom odd status [feature]".
// A diferencia de ParseCreateArgs y ParsePromoteArgs, el positional feature
// es opcional (REQ-19.6): sin él, el comando informa de todos los
// documentos vivos existentes bajo odd/tasks/.
func ParseStatusArgs(args []string) (StatusArgs, error) {
	const cmd = "axiom odd status"
	parsed := StatusArgs{CWD: "."}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--json":
			parsed.JSON = true
		case "--check-mirror":
			parsed.CheckMirror = true
		case "--cwd":
			value, err := nextFlagValue(args, i, cmd, arg)
			if err != nil {
				return StatusArgs{}, err
			}
			parsed.CWD = value
			i++
		default:
			if strings.HasPrefix(arg, "-") {
				return StatusArgs{}, fmt.Errorf("%s: bandera desconocida %q", cmd, arg)
			}
			if parsed.Feature != "" {
				return StatusArgs{}, fmt.Errorf("%s: argumento inesperado %q", cmd, arg)
			}
			parsed.Feature = arg
		}
	}

	return parsed, nil
}

// ParsePromoteArgs interpreta los argumentos de "axiom odd promote
// <feature>" (REQ-19.8): --name sobrescribe el nombre de cambio derivado de
// feature, --dry-run previsualiza sin escribir, --intent y --type se
// reenvían tal cual al andamiador de incrementos SDD.
func ParsePromoteArgs(args []string) (PromoteArgs, error) {
	const cmd = "axiom odd promote"
	parsed := PromoteArgs{CWD: "."}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--dry-run":
			parsed.DryRun = true
		case "--cwd":
			value, err := nextFlagValue(args, i, cmd, arg)
			if err != nil {
				return PromoteArgs{}, err
			}
			parsed.CWD = value
			i++
		case "--name":
			value, err := nextFlagValue(args, i, cmd, arg)
			if err != nil {
				return PromoteArgs{}, err
			}
			parsed.Name = value
			i++
		case "--intent":
			value, err := nextFlagValue(args, i, cmd, arg)
			if err != nil {
				return PromoteArgs{}, err
			}
			parsed.Intent = value
			i++
		case "--type":
			value, err := nextFlagValue(args, i, cmd, arg)
			if err != nil {
				return PromoteArgs{}, err
			}
			parsed.Type = value
			i++
		default:
			if strings.HasPrefix(arg, "-") {
				return PromoteArgs{}, fmt.Errorf("%s: bandera desconocida %q", cmd, arg)
			}
			if parsed.Feature != "" {
				return PromoteArgs{}, fmt.Errorf("%s: argumento inesperado %q", cmd, arg)
			}
			parsed.Feature = arg
		}
	}

	if parsed.Feature == "" {
		return PromoteArgs{}, fmt.Errorf("%s: falta el nombre de la feature como argumento posicional", cmd)
	}

	return parsed, nil
}

// nextFlagValue devuelve el valor que acompaña a flag en la posición i de
// args, o un error si falta o si el siguiente token parece otra bandera.
// Compartido por los tres parseadores de este fichero para no repetir la
// misma comprobación en cada bandera con valor [skill axiom-idiomatic-error-wrapping:
// mensaje descriptivo, sin causa subyacente que envolver aquí].
func nextFlagValue(args []string, i int, cmd, flag string) (string, error) {
	if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
		return "", fmt.Errorf("%s: la bandera %q requiere un valor", cmd, flag)
	}
	return args[i+1], nil
}

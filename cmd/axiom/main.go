package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/app"
	"github.com/gentleman-programming/gentle-ai/v2/internal/autoskill"
	"github.com/gentleman-programming/gentle-ai/v2/internal/cli"
	"github.com/gentleman-programming/gentle-ai/v2/internal/dashboard"
	"github.com/gentleman-programming/gentle-ai/v2/internal/handoff"
	"github.com/gentleman-programming/gentle-ai/v2/internal/hub"
	"github.com/gentleman-programming/gentle-ai/v2/internal/livingdoc"
	"github.com/gentleman-programming/gentle-ai/v2/internal/multirole"
	"github.com/gentleman-programming/gentle-ai/v2/internal/odd"
	"github.com/gentleman-programming/gentle-ai/v2/internal/semantic"
	"github.com/gentleman-programming/gentle-ai/v2/internal/update"
	"github.com/gentleman-programming/gentle-ai/v2/internal/workspace"
	"github.com/mattn/go-isatty"
)

func init() {
	// Desacoplar el comprobador de actualizaciones del upstream de Gentle AI:
	// Axiom es un fork independiente (IGutierrezZ/axiom) y no debe comparar
	// su versión contra Gentleman-Programming/gentle-ai ni forzar upgrades ajenos.
	for i, t := range update.Tools {
		if t.Name == "gentle-ai" {
			update.Tools[i] = update.ToolInfo{
				Name:          "axiom",
				Owner:         "IGutierrezZ",
				Repo:          "axiom",
				DetectCmd:     nil, // versión resuelta desde build-time (app.Version)
				VersionPrefix: "v",
				InstallMethod: update.InstallBinary,
				GoImportPath:  "github.com/IGutierrezZ/axiom/cmd/axiom",
			}
		}
	}
}

const (
	Version   = "v0.1.0"
	Platform  = "axiom"
	GitCommit = "dev"
)

func printHelp() {
	help := `Axiom — Plataforma de Desarrollo SDD Multi-Rol y Multi-Repositorio

USO:
  axiom <comando> [subcomando] [argumentos]
  axiom [banderas]
  axiom                Inicia la TUI interactiva si se ejecuta en un terminal interactivo (TTY)

COMANDOS DE GOBERNANZA Y WORKSPACE:
  init                 Inicializa un proyecto con axiom.yaml, andamiaje base y lo registra en el Hub
  change create        Crea un nuevo incremento/cambio SDD con plantilla proposal.md en español
  project list         Lista los proyectos registrados en el Hub global (~/.axiom/workspaces.json)
  project switch       Conmuta el proyecto activo por defecto
  project add          Registra un proyecto existente en el Hub
  project remove       Desvincula un proyecto del catálogo global
  workspace validate   Valida la configuración de axiom.yaml y la topología de repositorios
  handoff show         Muestra el relevo activo de un cambio
  handoff create       Genera una plantilla canónica de relevo (handoff.md)
  handoff validate     Valida la consistencia semántica y transición de fases de un relevo
  role list            Lista los roles asignados en el diseño y su política de compuerta
  role status          Muestra el progreso de tareas y verificación de cada rol
  role barrier         Evalúa la barrera de sincronización multi-rol antes de archivar o abrir PR
  skill scan           Detecta tecnologías y mina patrones locales, depositando candidatos en el buzón
  skill list           Lista las skills activas o propuestas pendientes en el buzón transitorio (--inbox)
  skill approve        Aprueba e instala formalmente una skill desde el buzón transitorio a skills/
  skill reject         Descarta y purga una propuesta del buzón transitorio
  semantic status      Diagnostica los conectores semánticos (Serena, CodeGraph, AST) y salud del workspace
  semantic symbols     Consulta y filtra símbolos de código (struct, interface, func, method)
  semantic inspect     Inspecciona el grafo de dependencias entre paquetes del workspace
  archive sync         Sincroniza y regenera el catálogo maestro openspec/INDEX.md desde openspec/specs/
  archive list         Lista las especificaciones vivas consolidadas y sus versiones
  archive show         Muestra el contenido Markdown de una especificación viva por dominio
  archive coldstart    Sintetiza una especificación viva inicial a partir de un cambio archivado
  sdd status           Consulta el estado de fases y artefactos de un cambio SDD (--json, --instructions)
  sdd continue         Calcula y emite la siguiente acción autorizada del despachador SDD
  sdd attempt          Gestiona el presupuesto y libro mayor de intentos de ejecución (acquire/settle)
  sdd verify-validate  Valida un reporte de verificación contra las especificaciones activas
  sdd archive-compose  Compone el reporte de archivado formal y actualiza las especificaciones vivas
  odd create           Crea un documento vivo ODD para una nueva feature del carril ágil
  odd status           Consulta el progreso de los documentos vivos ODD (--json, --check-mirror)
  odd promote          Promueve un documento vivo ODD a una propuesta SDD sembrada (--dry-run, --name)
  review               Gestiona el ciclo de revisión formal RDD (start, resume, step, mode, validate)
  ui                   Inicia el servidor local y abre el dashboard web interactivo

COMANDOS DE GESTIÓN DE AGENTES Y TUI:
  tui                  Abre la interfaz gráfica interactiva de terminal (TUI) de Axiom
  install              Instala agentes y configura el ecosistema en la máquina
  sync                 Sincroniza y re-aplica configuraciones, skills y reglas en los agentes
  upgrade              Actualiza herramientas y componentes gestionados del ecosistema
  doctor               Ejecuta diagnósticos de salud del ecosistema y herramientas instaladas
  backup               Lista y gestiona los respaldos de configuración
  restore              Restaura un respaldo previo de configuración
  uninstall            Desinstala componentes o plugins gestionados de agentes
  version              Muestra la versión e información de compilación
  help                 Muestra esta ayuda

BANDERAS:
  --version, -v        Muestra la versión de Axiom
  --help, -h           Muestra esta ayuda

Ejemplos:
  axiom
  axiom tui
  axiom init --name "MiProyecto"
  axiom sdd status mi-cambio --json
  axiom sdd continue mi-cambio
  axiom odd status
  axiom install claude-code
  axiom sync
  axiom doctor
  axiom ui
`
	fmt.Print(help)
}

var isattyFn = isTerminal

func isTerminal(fd uintptr) bool {
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

func runBackup(args []string, stdout io.Writer) {
	manifests := app.ListBackups()
	if len(manifests) == 0 {
		fmt.Fprintln(stdout, "No hay respaldos registrados en ~/.axiom/backups/ ni en ~/.gentle-ai/backups/")
		return
	}
	fmt.Fprintf(stdout, "Respaldos registrados (%d):\n", len(manifests))
	for _, m := range manifests {
		desc := m.Description
		if desc == "" {
			desc = "Sin descripción"
		}
		pin := ""
		if m.Pinned {
			pin = " [PIN]"
		}
		fmt.Fprintf(stdout, " - %s (%s)%s: %s\n", m.ID, m.CreatedAt.Format("2006-01-02 15:04:05"), pin, desc)
	}
}

func printVersion() {
	fmt.Printf("%s version %s (%s/%s) commit:%s\n", Platform, Version, runtime.GOOS, runtime.GOARCH, GitCommit)
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func main() {
	cli.AppVersion = Version
	app.Version = Version
	if len(os.Args) < 2 {
		if isattyFn(os.Stdin.Fd()) && isattyFn(os.Stdout.Fd()) {
			if err := app.RunArgs([]string{}, os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		}
		printHelp()
		os.Exit(0)
	}

	arg1 := os.Args[1]

	switch arg1 {
	case "--version", "-v", "version":
		printVersion()
		os.Exit(0)

	case "--help", "-h", "help":
		printHelp()
		os.Exit(0)

	case "tui":
		if err := app.RunArgs([]string{}, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)

	case "install", "sync", "upgrade", "update", "doctor", "restore", "uninstall":
		if err := app.RunArgs(os.Args[1:], os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)

	case "backup":
		runBackup(os.Args[2:], os.Stdout)
		os.Exit(0)

	case "init":
		runInit(os.Args[2:])

	case "project":
		if len(os.Args) < 3 {
			fmt.Println("Error: subcomando de 'project' requerido. Opciones: list, switch, add, remove")
			os.Exit(1)
		}

		subCmd := os.Args[2]
		switch subCmd {
		case "list":
			runProjectList(os.Args[3:])
		case "switch":
			runProjectSwitch(os.Args[3:])
		case "add":
			runProjectAdd(os.Args[3:])
		case "remove":
			runProjectRemove(os.Args[3:])
		default:
			fmt.Printf("Error: subcomando '%s' no reconocido para project. Usa 'axiom project [list|switch|add|remove]'.\n", subCmd)
			os.Exit(1)
		}

	case "change":
		exitCode := runChange(os.Args[2:], os.Stdout, os.Stderr)
		os.Exit(exitCode)

	case "workspace":

		if len(os.Args) < 3 {
			fmt.Println("Error: subcomando de 'workspace' requerido. Opciones: validate")
			os.Exit(1)
		}

		subCmd := os.Args[2]
		switch subCmd {
		case "validate":
			runWorkspaceValidate(os.Args[3:])
		default:
			fmt.Printf("Error: subcomando '%s' no reconocido para workspace. Usa 'axiom workspace validate'.\n", subCmd)
			os.Exit(1)
		}

	case "handoff":
		if len(os.Args) < 3 {
			fmt.Println("Error: subcomando de 'handoff' requerido. Opciones: show, create, validate")
			os.Exit(1)
		}

		subCmd := os.Args[2]
		switch subCmd {
		case "show":
			runHandoffShow(os.Args[3:])
		case "create":
			runHandoffCreate(os.Args[3:])
		case "validate":
			runHandoffValidate(os.Args[3:])
		default:
			fmt.Printf("Error: subcomando '%s' no reconocido para handoff. Usa 'axiom handoff [show|create|validate]'.\n", subCmd)
			os.Exit(1)
		}

	case "role":
		if len(os.Args) < 3 {
			fmt.Println("Error: subcomando de 'role' requerido. Opciones: list, status, barrier")
			os.Exit(1)
		}

		subCmd := os.Args[2]
		switch subCmd {
		case "list":
			runRoleList(os.Args[3:])
		case "status":
			runRoleStatus(os.Args[3:])
		case "barrier":
			runRoleBarrier(os.Args[3:])
		default:
			fmt.Printf("Error: subcomando '%s' no reconocido para role. Usa 'axiom role [list|status|barrier]'.\n", subCmd)
			os.Exit(1)
		}

	case "skill":
		if len(os.Args) < 3 {
			fmt.Println("Error: subcomando de 'skill' requerido. Opciones: scan, list, approve, reject")
			os.Exit(1)
		}

		subCmd := os.Args[2]
		switch subCmd {
		case "scan":
			runSkillScan(os.Args[3:])
		case "list":
			runSkillList(os.Args[3:])
		case "approve":
			runSkillApprove(os.Args[3:])
		case "reject":
			runSkillReject(os.Args[3:])
		default:
			fmt.Printf("Error: subcomando '%s' no reconocido para skill. Usa 'axiom skill [scan|list|approve|reject]'.\n", subCmd)
			os.Exit(1)
		}

	case "semantic":
		if len(os.Args) < 3 {
			fmt.Println("Error: subcomando de 'semantic' requerido. Opciones: status, symbols, inspect")
			os.Exit(1)
		}

		subCmd := os.Args[2]
		switch subCmd {
		case "status":
			runSemanticStatus(os.Args[3:])
		case "symbols":
			runSemanticSymbols(os.Args[3:])
		case "inspect":
			runSemanticInspect(os.Args[3:])
		default:
			fmt.Printf("Error: subcomando '%s' no reconocido para semantic. Usa 'axiom semantic [status|symbols|inspect]'.\n", subCmd)
			os.Exit(1)
		}

	case "archive":
		if len(os.Args) < 3 {
			fmt.Println("Error: subcomando de 'archive' requerido. Opciones: sync, list, show, coldstart")
			os.Exit(1)
		}
		subCmd := os.Args[2]
		switch subCmd {
		case "sync":
			runArchiveSync(os.Args[3:])
		case "list":
			runArchiveList(os.Args[3:])
		case "show":
			runArchiveShow(os.Args[3:])
		case "coldstart":
			runArchiveColdStart(os.Args[3:])
		default:
			fmt.Printf("Error: subcomando '%s' no reconocido para archive. Usa 'axiom archive [sync|list|show|coldstart]'.\n", subCmd)
			os.Exit(1)
		}

	case "ui":
		runUI(os.Args[2:])

	case "odd":
		os.Exit(runODD(os.Args[2:], os.Stdout, os.Stderr))

	case "sdd":
		os.Exit(runSDD(os.Args[2:], os.Stdout, os.Stderr))
	case "sdd-status":
		os.Exit(runSDD(append([]string{"status"}, os.Args[2:]...), os.Stdout, os.Stderr))
	case "sdd-continue":
		os.Exit(runSDD(append([]string{"continue"}, os.Args[2:]...), os.Stdout, os.Stderr))
	case "sdd-attempt":
		os.Exit(runSDD(append([]string{"attempt"}, os.Args[2:]...), os.Stdout, os.Stderr))
	case "sdd-verify-validate":
		os.Exit(runSDD(append([]string{"verify-validate"}, os.Args[2:]...), os.Stdout, os.Stderr))
	case "sdd-archive-compose":
		os.Exit(runSDD(append([]string{"archive-compose"}, os.Args[2:]...), os.Stdout, os.Stderr))
	case "sdd-task-result":
		os.Exit(runSDD(append([]string{"task-result"}, os.Args[2:]...), os.Stdout, os.Stderr))
	case "sdd-preflight-hook":
		os.Exit(runSDD(append([]string{"preflight-hook"}, os.Args[2:]...), os.Stdout, os.Stderr))

	case "review":
		os.Exit(runReview(os.Args[2:], os.Stdout, os.Stderr, false))
	case "review-start":
		os.Exit(runReview(append([]string{"start"}, os.Args[2:]...), os.Stdout, os.Stderr, true))
	case "review-resume":
		os.Exit(runReview(append([]string{"resume"}, os.Args[2:]...), os.Stdout, os.Stderr, true))
	case "review-step":
		os.Exit(runReview(append([]string{"step"}, os.Args[2:]...), os.Stdout, os.Stderr, true))
	case "review-bundle-export":
		os.Exit(runReview(append([]string{"bundle-export"}, os.Args[2:]...), os.Stdout, os.Stderr, true))
	case "review-bundle-import":
		os.Exit(runReview(append([]string{"bundle-import"}, os.Args[2:]...), os.Stdout, os.Stderr, true))
	case "review-validate":
		os.Exit(runReview(append([]string{"validate"}, os.Args[2:]...), os.Stdout, os.Stderr, true))

	// These four existed only in internal/app, so they were reachable through
	// the deprecated `gentle-ai` wrapper and not through the canonical binary.
	// Nobody noticed because no CI step exercised cmd/axiom. `skill-registry`
	// and `bench-model-picker` delegate to app because their implementations
	// are unexported there.
	case "codegraph":
		os.Exit(runSimpleCommand(cli.RunCodeGraph, os.Args[2:], os.Stdout, os.Stderr))
	case "telemetry":
		os.Exit(runSimpleCommand(cli.RunTelemetry, os.Args[2:], os.Stdout, os.Stderr))
	case "skill-registry":
		os.Exit(runSimpleCommand(
			func(args []string, stdout io.Writer) error {
				return app.RunArgs(append([]string{"skill-registry"}, args...), stdout)
			},
			os.Args[2:], os.Stdout, os.Stderr))
	// Only a build carrying `-tags bench_fixture` implements this verb; every
	// other build falls through app's own switch and refuses it as an unknown
	// command, which is what lets the benchmark report `unsupported` instead of
	// fabricating a pass. Delegating preserves that distinction here too.
	case "bench-model-picker":
		os.Exit(runSimpleCommand(
			func(args []string, stdout io.Writer) error {
				return app.RunArgs(append([]string{"bench-model-picker"}, args...), stdout)
			},
			os.Args[2:], os.Stdout, os.Stderr))

	default:
		fmt.Printf("Error: comando '%s' no reconocido.\n\n", arg1)
		printHelp()
		os.Exit(1)
	}
}

func runChange(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "Uso: axiom change <subcomando> [argumentos]")
		fmt.Fprintln(stdout, "\nSubcomandos disponibles:")
		fmt.Fprintln(stdout, "  create, new      Crea un nuevo incremento/cambio SDD con plantilla proposal.md en español")
		if len(args) < 1 {
			return 1
		}
		return 0
	}

	subCmd := args[0]
	subArgs := args[1:]

	switch subCmd {
	case "create", "new":
		if len(subArgs) > 0 && (subArgs[0] == "--help" || subArgs[0] == "-h") {
			fmt.Fprintln(stdout, "Uso: axiom change create <nombre> [--intent <desc>] [--type <tipo>] [--cwd <ruta>]")
			fmt.Fprintln(stdout, "\nBanderas:")
			fmt.Fprintln(stdout, "  --intent, -i     Propósito o descripción del incremento/cambio")
			fmt.Fprintln(stdout, "  --type, -t       Tipo de cambio (feature, fix, refactor, architecture)")
			fmt.Fprintln(stdout, "  --cwd            Directorio raíz del proyecto")
			return 0
		}
		if err := runChangeCreate(subArgs, stdout, stderr); err != nil {
			return 1
		}
		return 0
	default:
		fmt.Fprintf(stderr, "Error: subcomando '%s' no reconocido para change. Usa 'axiom change [create|new]'.\n", subCmd)
		return 1
	}
}

func runChangeCreate(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("change create", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var intent string
	var changeType string
	var cwd string

	fs.StringVar(&intent, "intent", "", "Propósito o descripción del incremento/cambio")
	fs.StringVar(&intent, "i", "", "Propósito o descripción del incremento/cambio (abreviado)")
	fs.StringVar(&changeType, "type", "feature", "Tipo de cambio (feature, fix, refactor, architecture)")
	fs.StringVar(&changeType, "t", "feature", "Tipo de cambio (abreviado)")
	fs.StringVar(&cwd, "cwd", ".", "Directorio raíz del proyecto")

	var flagArgs []string
	var posArgs []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
			if !strings.Contains(arg, "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				flagArgs = append(flagArgs, args[i])
			}
		} else {
			posArgs = append(posArgs, arg)
		}
	}

	if err := fs.Parse(flagArgs); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	if len(posArgs) == 0 {
		fmt.Fprintln(stderr, "Error: nombre del cambio requerido. Uso: axiom change create <nombre> [--intent <desc>] [--type <tipo>]")
		return fmt.Errorf("nombre del cambio requerido")
	}

	changeName := posArgs[0]
	svc := dashboard.NewService(cwd)
	res, err := svc.CreateIncrement(dashboard.CreateIncrementRequest{
		Name:   changeName,
		Intent: intent,
		Type:   changeType,
	})
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return err
	}

	fmt.Fprintf(stdout, "Incremento '%s' creado correctamente en %s\n", res.Name, res.Path)
	return nil
}

func runWorkspaceValidate(args []string) {
	fs := flag.NewFlagSet("workspace validate", flag.ExitOnError)
	pathFlag := fs.String("path", ".", "Ruta a la carpeta maestra del espacio de trabajo")
	if err := fs.Parse(args); err != nil {
		fmt.Printf("Error al analizar banderas: %v\n", err)
		os.Exit(1)
	}

	baseDir, err := filepath.Abs(*pathFlag)
	if err != nil {
		fmt.Printf("Error al resolver ruta base '%s': %v\n", *pathFlag, err)
		os.Exit(1)
	}

	configFile := filepath.Join(baseDir, "axiom.yaml")
	cfg, err := workspace.LoadConfig(configFile)
	if err != nil {
		fmt.Printf("[ERROR] No se pudo cargar la configuración de Axiom:\n  %v\n", err)
		os.Exit(1)
	}

	report, err := workspace.Validate(workspace.DefaultFS(), cfg, baseDir)
	if err != nil {
		fmt.Printf("[ERROR] Error inesperado en el motor de validación:\n  %v\n", err)
		os.Exit(1)
	}

	if !report.Valid {
		fmt.Printf("[ERROR] Espacio de trabajo NO CONFORME (Topología: %s)\n", report.Topology)
		fmt.Printf("Directorio evaluado: %s\n\n", report.WorkspaceRoot)
		fmt.Println("Errores encontrados:")
		for i, e := range report.Errors {
			fmt.Printf("  %d. %s\n", i+1, e)
		}
		if len(report.Warnings) > 0 {
			fmt.Println("\nAdvertencias:")
			for i, w := range report.Warnings {
				fmt.Printf("  %d. %s\n", i+1, w)
			}
		}
		fmt.Println("\nResultado: NON-COMPLIANT")
		os.Exit(1)
	}

	fmt.Printf("[OK] Espacio de trabajo conforme (Topología: %s)\n", report.Topology)
	fmt.Printf("  - Proyecto: %s\n", cfg.Workspace.Name)
	fmt.Printf("  - Directorio base: %s\n", report.WorkspaceRoot)
	if cfg.Workspace.SpecsRepository != "" && cfg.Workspace.SpecsRepository != "." {
		fmt.Printf("  - Repositorio canónico de specs: %s [OK]\n", filepath.Join(report.WorkspaceRoot, cfg.Workspace.SpecsRepository))
	} else {
		fmt.Println("  - Repositorio canónico de specs: Embebido en raíz [OK]")
	}
	fmt.Printf("  - Roles validados (%d):\n", len(cfg.Roles))
	for roleKey, role := range cfg.Roles {
		fmt.Printf("      * %s (%s) — %d repositorio(s)\n", roleKey, role.Name, len(role.Repositories))
	}
	fmt.Printf("  - Total rutas verificadas: %d\n", len(report.CheckedPaths))
	fmt.Println("\nResultado: COMPLIANT")
	os.Exit(0)
}

func runHandoffShow(args []string) {
	fs := flag.NewFlagSet("handoff show", flag.ExitOnError)
	changeFlag := fs.String("change", "", "Nombre del cambio (ej. inc-02-structured-handoffs-lifecycle)")
	pathFlag := fs.String("path", ".", "Ruta base del proyecto o repositorio")
	if err := fs.Parse(args); err != nil {
		fmt.Printf("Error al analizar banderas: %v\n", err)
		os.Exit(1)
	}

	handoffPath, err := resolveHandoffFile(*pathFlag, *changeFlag)
	if err != nil {
		fmt.Printf("[ERROR] %v\n", err)
		os.Exit(1)
	}

	h, err := handoff.ParseFile(handoffPath)
	if err != nil {
		fmt.Printf("[ERROR] No se pudo leer el archivo de handoff:\n  %v\n", err)
		os.Exit(1)
	}

	fmt.Println("================================================================================")
	fmt.Printf("Axiom Handoff: %s (Estado: %s)\n", h.Metadata.Change, strings.ToUpper(string(h.Metadata.Status)))
	fmt.Println("================================================================================")
	fmt.Printf("Transición:   %s -> %s\n", h.Metadata.FromPhase, h.Metadata.ToPhase)
	fmt.Printf("Rol Emisor:   %s\n", h.Metadata.FromRole)
	fmt.Printf("Rol Receptor: %s\n", h.Metadata.ToRole)
	fmt.Printf("Timestamp:    %s\n", h.Metadata.Timestamp.Format(time.RFC3339))
	fmt.Printf("Ubicación:    %s\n\n", handoffPath)

	fmt.Printf("[%s]\n%s\n\n", handoff.HeaderExecutiveSummary, h.Sections.ExecutiveSummary)
	fmt.Printf("[%s]\n%s\n\n", handoff.HeaderArtifacts, h.Sections.Artifacts)
	fmt.Printf("[%s]\n%s\n\n", handoff.HeaderDecisions, h.Sections.Decisions)
	fmt.Printf("[%s]\n%s\n\n", handoff.HeaderRisksAndBlockers, h.Sections.RisksAndBlockers)
	fmt.Printf("[%s]\n%s\n", handoff.HeaderDirectInstructions, h.Sections.DirectInstructions)
	fmt.Println("================================================================================")
	os.Exit(0)
}

func runHandoffCreate(args []string) {
	fs := flag.NewFlagSet("handoff create", flag.ExitOnError)
	changeFlag := fs.String("change", "", "Nombre del cambio")
	fromPhaseFlag := fs.String("from", "", "Fase de origen (explore, propose, spec, design, tasks, apply, verify)")
	toPhaseFlag := fs.String("to", "", "Fase de destino (propose, spec, design, tasks, apply, verify, archive)")
	fromRoleFlag := fs.String("from-role", "", "Rol emisor del relevo")
	toRoleFlag := fs.String("to-role", "", "Rol receptor del relevo")
	statusFlag := fs.String("status", "ready", "Estado del relevo (ready, blocked, needs_clarification)")
	pathFlag := fs.String("path", ".", "Ruta base del proyecto o repositorio")

	if err := fs.Parse(args); err != nil {
		fmt.Printf("Error al analizar banderas: %v\n", err)
		os.Exit(1)
	}

	if *changeFlag == "" || *fromPhaseFlag == "" || *toPhaseFlag == "" || *fromRoleFlag == "" || *toRoleFlag == "" {
		fmt.Println("[ERROR] Parámetros obligatorios faltantes. Se requiere --change, --from, --to, --from-role y --to-role.")
		os.Exit(1)
	}

	baseDir, err := filepath.Abs(*pathFlag)
	if err != nil {
		fmt.Printf("Error al resolver ruta base: %v\n", err)
		os.Exit(1)
	}

	targetPath := filepath.Join(baseDir, "openspec", "changes", *changeFlag, "handoff.md")

	h := &handoff.Handoff{
		Metadata: handoff.Metadata{
			Change:    *changeFlag,
			FromPhase: handoff.Phase(*fromPhaseFlag),
			ToPhase:   handoff.Phase(*toPhaseFlag),
			FromRole:  *fromRoleFlag,
			ToRole:    *toRoleFlag,
			Timestamp: time.Now().UTC(),
			Status:    handoff.Status(*statusFlag),
		},
		Sections: handoff.Sections{
			ExecutiveSummary:   fmt.Sprintf("Culminada fase %s. Preparado el relevo para la fase %s.", *fromPhaseFlag, *toPhaseFlag),
			Artifacts:          fmt.Sprintf("- openspec/changes/%s/*", *changeFlag),
			Decisions:          "Acuerdos y decisiones tomadas documentadas en los artefactos correspondientes.",
			RisksAndBlockers:   "Ninguno reportado.",
			DirectInstructions: fmt.Sprintf("Iniciar fase %s siguiendo las especificaciones aprobadas.", *toPhaseFlag),
		},
	}

	if err := handoff.WriteFile(targetPath, h); err != nil {
		fmt.Printf("[ERROR] No se pudo escribir el archivo de handoff:\n  %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[OK] Plantilla de relevo creada exitosamente:\n  Ruta: %s\n  Transición: %s -> %s\n  Emisor: %s | Receptor: %s\n",
		targetPath, *fromPhaseFlag, *toPhaseFlag, *fromRoleFlag, *toRoleFlag)
	os.Exit(0)
}

func runHandoffValidate(args []string) {
	fs := flag.NewFlagSet("handoff validate", flag.ExitOnError)
	changeFlag := fs.String("change", "", "Nombre del cambio")
	pathFlag := fs.String("path", ".", "Ruta base del proyecto o repositorio")

	if err := fs.Parse(args); err != nil {
		fmt.Printf("Error al analizar banderas: %v\n", err)
		os.Exit(1)
	}

	baseDir, err := filepath.Abs(*pathFlag)
	if err != nil {
		fmt.Printf("Error al resolver ruta base: %v\n", err)
		os.Exit(1)
	}

	handoffPath, err := resolveHandoffFile(baseDir, *changeFlag)
	if err != nil {
		fmt.Printf("[ERROR] %v\n", err)
		os.Exit(1)
	}

	h, err := handoff.ParseFile(handoffPath)
	if err != nil {
		fmt.Printf("[ERROR] HANDOFF INVALID (Error de sintaxis o formato):\n  %v\n", err)
		os.Exit(1)
	}

	// Cargar configuración de workspace opcionalmente si existe axiom.yaml
	var wsConfig *workspace.WorkspaceConfig
	configPath := filepath.Join(baseDir, "axiom.yaml")
	if _, err := os.Stat(configPath); err == nil {
		if loadedCfg, err := workspace.LoadConfig(configPath); err == nil {
			wsConfig = loadedCfg
		}
	}

	if err := handoff.Validate(h, wsConfig); err != nil {
		fmt.Printf("[ERROR] HANDOFF INVALID (Validación semántica fallida):\n  %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[OK] HANDOFF VALID: %s (%s -> %s)\n", h.Metadata.Change, h.Metadata.FromPhase, h.Metadata.ToPhase)
	fmt.Printf("  - Estado: %s\n", h.Metadata.Status)
	fmt.Printf("  - Emisor: %s | Receptor: %s\n", h.Metadata.FromRole, h.Metadata.ToRole)
	fmt.Printf("  - Archivo: %s\n", handoffPath)
	os.Exit(0)
}

func runRoleList(args []string) {
	fs := flag.NewFlagSet("role list", flag.ExitOnError)
	changeFlag := fs.String("change", "", "Nombre del cambio")
	pathFlag := fs.String("path", ".", "Ruta base del proyecto o repositorio")
	if err := fs.Parse(args); err != nil {
		fmt.Printf("Error al analizar banderas: %v\n", err)
		os.Exit(1)
	}

	baseDir, changeName, changeDir, err := resolveChangeDir(*pathFlag, *changeFlag)
	if err != nil {
		fmt.Printf("[ERROR] %v\n", err)
		os.Exit(1)
	}

	var wsConfig *workspace.WorkspaceConfig
	cfgPath := filepath.Join(baseDir, "axiom.yaml")
	if _, err := os.Stat(cfgPath); err == nil {
		wsConfig, _ = workspace.LoadConfig(cfgPath)
	}

	designFile := filepath.Join(changeDir, "design.md")
	roles, err := multirole.DetectRoles(designFile, wsConfig)
	if err != nil {
		fmt.Printf("[ERROR] No se pudieron detectar los roles del cambio %q:\n  %v\n", changeName, err)
		os.Exit(1)
	}

	fmt.Println("================================================================================")
	fmt.Printf("Axiom Roles Participantes: %s\n", changeName)
	fmt.Println("================================================================================")
	fmt.Printf("Total de roles asignados: %d\n\n", len(roles))

	for i, r := range roles {
		policyLabel := strings.ToUpper(string(r.GatePolicy))
		fmt.Printf("  %d. Rol: %s [%s]\n", i+1, r.Role, policyLabel)
		if r.Name != "" {
			fmt.Printf("     Nombre descriptivo: %s\n", r.Name)
		}
		if len(r.Repositories) > 0 {
			fmt.Printf("     Repositorios: %s\n", strings.Join(r.Repositories, ", "))
		}
		if len(r.Deliverables) > 0 {
			fmt.Printf("     Entregables esperados: %s\n", strings.Join(r.Deliverables, ", "))
		}
		fmt.Println()
	}
	fmt.Println("================================================================================")
	os.Exit(0)
}

func runRoleStatus(args []string) {
	fs := flag.NewFlagSet("role status", flag.ExitOnError)
	changeFlag := fs.String("change", "", "Nombre del cambio")
	pathFlag := fs.String("path", ".", "Ruta base del proyecto o repositorio")
	if err := fs.Parse(args); err != nil {
		fmt.Printf("Error al analizar banderas: %v\n", err)
		os.Exit(1)
	}

	baseDir, changeName, changeDir, err := resolveChangeDir(*pathFlag, *changeFlag)
	if err != nil {
		fmt.Printf("[ERROR] %v\n", err)
		os.Exit(1)
	}

	var wsConfig *workspace.WorkspaceConfig
	cfgPath := filepath.Join(baseDir, "axiom.yaml")
	if _, err := os.Stat(cfgPath); err == nil {
		wsConfig, _ = workspace.LoadConfig(cfgPath)
	}

	designFile := filepath.Join(changeDir, "design.md")
	roles, err := multirole.DetectRoles(designFile, wsConfig)
	if err != nil {
		fmt.Printf("[ERROR] No se pudieron detectar los roles del cambio %q:\n  %v\n", changeName, err)
		os.Exit(1)
	}

	report, err := multirole.EvaluateBarrier(changeDir, changeName, roles)
	if err != nil {
		fmt.Printf("[ERROR] Error al evaluar el estado de los roles:\n  %v\n", err)
		os.Exit(1)
	}

	fmt.Println("================================================================================")
	fmt.Printf("Axiom Estado de Roles: %s\n", changeName)
	fmt.Println("================================================================================")

	for i, r := range report.Roles {
		policyLabel := strings.ToUpper(string(r.Assignment.GatePolicy))
		fmt.Printf("  %d. Rol: %s [%s]\n", i+1, r.Assignment.Role, policyLabel)

		if r.TasksFound {
			fmt.Printf("     Tareas: %d/%d completadas (%.1f%%) — %d pendiente(s)\n",
				r.Tasks.Completed, r.Tasks.Total, r.Tasks.Percent, r.Tasks.Pending)
		} else {
			fmt.Println("     Tareas: [NO ENCONTRADO / PENDIENTE]")
		}

		if r.VerifyDone {
			fmt.Printf("     Verificación: [%s] [OK]\n", strings.ToUpper(r.Verdict))
		} else if r.Verdict == "missing" {
			fmt.Println("     Verificación: [PENDIENTE / NO EMITIDO]")
		} else {
			fmt.Printf("     Verificación: [%s] [FALLO]\n", strings.ToUpper(r.Verdict))
		}
		fmt.Println()
	}

	if len(report.Warnings) > 0 {
		fmt.Println("Advertencias:")
		for _, w := range report.Warnings {
			fmt.Printf("  - [AVISO] %s\n", w)
		}
		fmt.Println()
	}

	fmt.Println("================================================================================")
	os.Exit(0)
}

func runRoleBarrier(args []string) {
	fs := flag.NewFlagSet("role barrier", flag.ExitOnError)
	changeFlag := fs.String("change", "", "Nombre del cambio")
	pathFlag := fs.String("path", ".", "Ruta base del proyecto o repositorio")
	migrateDeferredFlag := fs.Bool("migrate-deferred", false, "Vuelca las tareas diferidas pendientes al incremento acumulativo e2e-cumulative")
	if err := fs.Parse(args); err != nil {
		fmt.Printf("Error al analizar banderas: %v\n", err)
		os.Exit(1)
	}

	baseDir, changeName, changeDir, err := resolveChangeDir(*pathFlag, *changeFlag)
	if err != nil {
		fmt.Printf("[ERROR] %v\n", err)
		os.Exit(1)
	}

	var wsConfig *workspace.WorkspaceConfig
	cfgPath := filepath.Join(baseDir, "axiom.yaml")
	if _, err := os.Stat(cfgPath); err == nil {
		wsConfig, _ = workspace.LoadConfig(cfgPath)
	}

	designFile := filepath.Join(changeDir, "design.md")
	roles, err := multirole.DetectRoles(designFile, wsConfig)
	if err != nil {
		fmt.Printf("[ERROR] No se pudieron detectar los roles del cambio %q:\n  %v\n", changeName, err)
		os.Exit(1)
	}

	report, err := multirole.EvaluateBarrier(changeDir, changeName, roles)
	if err != nil {
		fmt.Printf("[ERROR] Error inesperado en el motor de barrera:\n  %v\n", err)
		os.Exit(1)
	}

	if !report.Satisfied {
		fmt.Printf("[ERROR] BARRIER BLOCKED: La barrera de sincronización no se cumple para %q\n\n", changeName)
		fmt.Println("Motivos de bloqueo detectados:")
		for i, b := range report.Blockers {
			fmt.Printf("  %d. %s\n", i+1, b)
		}
		if len(report.Warnings) > 0 {
			fmt.Println("\nAdvertencias adicionales:")
			for _, w := range report.Warnings {
				fmt.Printf("  - %s\n", w)
			}
		}
		fmt.Println("\nResultado: BLOCKED (No autorizado para abrir PR ni mergear a main)")
		os.Exit(1)
	}

	fmt.Printf("[OK] BARRIER SATISFIED: Todos los roles obligatorios (blocking) han verificado con éxito para %q\n\n", changeName)
	for _, r := range report.Roles {
		if r.Assignment.GatePolicy == multirole.PolicyBlocking {
			fmt.Printf("  - [PASS] %s: %d/%d tareas completadas (100%%) | Verificación: %s\n",
				r.Assignment.Role, r.Tasks.Completed, r.Tasks.Total, strings.ToUpper(r.Verdict))
		}
	}

	if len(report.Warnings) > 0 {
		fmt.Println("\nAdvertencias de roles asíncronos / diferidos:")
		for _, w := range report.Warnings {
			fmt.Printf("  - %s\n", w)
		}
	}

	if len(report.DeferredTasks) > 0 {
		fmt.Printf("\nTareas diferidas capturadas con trazabilidad: %d tarea(s)\n", len(report.DeferredTasks))
		for _, dt := range report.DeferredTasks {
			fmt.Printf("  * [%s] %s\n", dt.Role, dt.TaskText)
		}

		if *migrateDeferredFlag {
			cumulativeFile := filepath.Join(baseDir, "openspec", "changes", "e2e-cumulative", "tasks.md")
			if err := multirole.MigrateDeferredTasks(cumulativeFile, report.DeferredTasks); err != nil {
				fmt.Printf("\n[AVISO] No se pudieron migrar las tareas diferidas: %v\n", err)
			} else {
				fmt.Printf("\n[OK] %d tarea(s) diferida(s) migradas exitosamente al acumulativo:\n  Ruta: %s\n",
					len(report.DeferredTasks), cumulativeFile)
			}
		} else {
			fmt.Println("\n(Consejo: Usa --migrate-deferred para volcar estas tareas al backlog continuo de QA)")
		}
	}

	fmt.Println("\nResultado: SATISFIED (Autorizado para despliegue en staging y PR a main)")
	os.Exit(0)
}

func resolveChangeDir(baseDir, change string) (string, string, string, error) {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", "", "", fmt.Errorf("ruta base inválida: %w", err)
	}

	changesDir := filepath.Join(absBase, "openspec", "changes")

	if change != "" {
		target := filepath.Join(changesDir, change)
		if _, err := os.Stat(target); err != nil {
			return "", "", "", fmt.Errorf("no existe el directorio del cambio %q en: %s", change, target)
		}
		return absBase, change, target, nil
	}

	entries, err := os.ReadDir(changesDir)
	if err != nil {
		return "", "", "", fmt.Errorf("no se pudo inspeccionar el directorio de cambios %q: %w", changesDir, err)
	}

	var foundDirs []string
	for _, e := range entries {
		if e.IsDir() && e.Name() != "archive" {
			foundDirs = append(foundDirs, e.Name())
		}
	}

	if len(foundDirs) == 0 {
		return "", "", "", fmt.Errorf("no se encontró ningún cambio activo bajo %s", changesDir)
	}
	if len(foundDirs) > 1 {
		return "", "", "", fmt.Errorf("se encontraron múltiples cambios activos (%s). Especifica el cambio con --change", strings.Join(foundDirs, ", "))
	}

	changeName := foundDirs[0]
	return absBase, changeName, filepath.Join(changesDir, changeName), nil
}

func resolveHandoffFile(baseDir, change string) (string, error) {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("ruta base inválida: %w", err)
	}

	if change != "" {
		target := filepath.Join(absBase, "openspec", "changes", change, "handoff.md")
		if _, err := os.Stat(target); err != nil {
			return "", fmt.Errorf("no existe archivo de relevo para el cambio %q en: %s", change, target)
		}
		return target, nil
	}

	// Buscar en openspec/changes los cambios activos
	changesDir := filepath.Join(absBase, "openspec", "changes")
	entries, err := os.ReadDir(changesDir)
	if err != nil {
		return "", fmt.Errorf("no se pudo inspeccionar el directorio de cambios %q: %w", changesDir, err)
	}

	var foundPaths []string
	for _, e := range entries {
		if e.IsDir() && e.Name() != "archive" {
			hPath := filepath.Join(changesDir, e.Name(), "handoff.md")
			if _, err := os.Stat(hPath); err == nil {
				foundPaths = append(foundPaths, hPath)
			}
		}
	}

	if len(foundPaths) == 0 {
		return "", fmt.Errorf("no se encontró ningún archivo handoff.md en los cambios activos bajo %s", changesDir)
	}
	if len(foundPaths) > 1 {
		return "", fmt.Errorf("se encontraron múltiples relevos activos. Por favor especifica el cambio con --change")
	}

	return foundPaths[0], nil
}

func runUI(args []string) {
	fs := flag.NewFlagSet("ui", flag.ExitOnError)
	portFlag := fs.Int("port", 8080, "Puerto de escucha para el servidor web (default: 8080)")
	noBrowserFlag := fs.Bool("no-browser", false, "No abrir automáticamente el navegador")
	pathFlag := fs.String("path", ".", "Ruta base del espacio de trabajo")

	if err := fs.Parse(args); err != nil {
		fmt.Printf("Error al analizar banderas: %v\n", err)
		os.Exit(1)
	}

	baseDir, err := filepath.Abs(*pathFlag)
	if err != nil {
		fmt.Printf("Error al resolver ruta base: %v\n", err)
		os.Exit(1)
	}

	hubMgr, err := hub.NewManager("")
	if err != nil {
		fmt.Printf("  (Aviso: No se pudo cargar el registro del hub: %v)\n", err)
	}

	// Si se invocó con '.' por defecto y no hay axiom.yaml en '.', resolver el workspace activo del Hub
	if *pathFlag == "." && hubMgr != nil {
		if !fileExists(filepath.Join(baseDir, "axiom.yaml")) {
			if active, err := hubMgr.GetActive(); err == nil && active != nil && fileExists(filepath.Join(active.Path, "axiom.yaml")) {
				baseDir = active.Path
			}
		}
	}

	svc := dashboard.NewServiceWithHub(baseDir, hubMgr)
	server := dashboard.NewServer(svc)

	actualPort, err := server.ListenAndServe(*portFlag)
	if err != nil {
		fmt.Printf("[ERROR] No se pudo iniciar el servidor web: %v\n", err)
		os.Exit(1)
	}

	url := fmt.Sprintf("http://127.0.0.1:%d", actualPort)

	fmt.Println("================================================================================")
	fmt.Println("             Axiom Enterprise — Dashboard Web Local (SDD)")
	fmt.Println("================================================================================")
	fmt.Printf("  Servidor HTTP activo en: %s\n", url)
	fmt.Println("  Espacio de Trabajo:     ", baseDir)
	if actualPort != *portFlag {
		fmt.Printf("  (Aviso: Puerto %d ocupado, reasignado a %d)\n", *portFlag, actualPort)
	}
	fmt.Println("  Presiona Ctrl+C para detener el servidor.")
	fmt.Println("================================================================================")

	if !*noBrowserFlag {
		go func() {
			time.Sleep(150 * time.Millisecond)
			if err := openBrowser(url); err != nil {
				fmt.Printf("  (Aviso: No se pudo abrir el navegador automáticamente: %v)\n", err)
			}
		}()
	}

	// Esperar señal de parada
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)
	<-stopChan

	fmt.Println("\nDeteniendo el servidor web de Axiom...")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		fmt.Printf("Error durante el cierre del servidor: %v\n", err)
	}
	fmt.Println("Servidor detenido correctamente.")
	os.Exit(0)
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func runSkillScan(args []string) {
	fs := flag.NewFlagSet("skill scan", flag.ExitOnError)
	pathFlag := fs.String("path", ".", "Ruta a la carpeta maestra del espacio de trabajo")
	roleFlag := fs.String("role", "", "Filtrar escaneo por un rol específico")
	offlineFlag := fs.Bool("offline", false, "Operar sin consultar la red, utilizando solo caché local y minería")

	if err := fs.Parse(args); err != nil {
		fmt.Printf("Error al analizar banderas: %v\n", err)
		os.Exit(1)
	}

	baseDir, err := filepath.Abs(*pathFlag)
	if err != nil {
		fmt.Printf("Error obteniendo ruta absoluta: %v\n", err)
		os.Exit(1)
	}

	manager := autoskill.NewManager(baseDir, nil, nil, nil)
	fmt.Println("================================================================================")
	fmt.Println("       Axiom — Escaneo de Autoskills & Minería Heurística de Repositorio")
	fmt.Println("================================================================================")
	fmt.Printf("  Espacio de Trabajo: %s\n", baseDir)
	if *roleFlag != "" {
		fmt.Printf("  Rol objetivo:       %s\n", *roleFlag)
	}
	modeStr := "Online (Registro midudev/autoskills & Minería)"
	if *offlineFlag {
		modeStr = "Offline (Caché & Minería)"
	}
	fmt.Printf("  Modo:               %s\n", modeStr)
	fmt.Println("  Analizando dependencias, configuraciones y código fuente...")
	fmt.Println("--------------------------------------------------------------------------------")

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	report, err := manager.Scan(ctx, *roleFlag, *offlineFlag)
	if err != nil {
		fmt.Printf("\nError ejecutando escaneo: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("  Tecnologías Detectadas: %s\n", strings.Join(report.DetectedTechnologies, ", "))
	fmt.Printf("  Nuevas Skills Propuestas: %d (%d de midudev auditado, %d de minería local)\n",
		len(report.SkillsProposed), report.RegistrySkillsCount, report.MinedSkillsCount)
	fmt.Printf("  Total en Buzón Transitorio: %d\n", report.TotalInInbox)

	if len(report.SkillsProposed) > 0 {
		fmt.Println("\nPropuestas añadidas al buzón (.axiom/skills/inbox/):")
		for _, p := range report.SkillsProposed {
			verifiedStr := "[SHA-256 VERIFICADO]"
			if !p.Metadata.Verified {
				verifiedStr = "[SIN VERIFICAR]"
			}
			fmt.Printf("  • %-30s | Origen: %-8s | %s | %s\n",
				p.Metadata.Name, p.Metadata.Origin, verifiedStr, p.Metadata.Justification)
		}
		fmt.Println("\nPara revisar y aprobar una skill:")
		fmt.Println("  axiom skill approve <nombre>")
	} else {
		fmt.Println("\nNo se detectaron nuevas directrices para añadir al buzón.")
	}
	fmt.Println("================================================================================")
}

func runSkillList(args []string) {
	fs := flag.NewFlagSet("skill list", flag.ExitOnError)
	pathFlag := fs.String("path", ".", "Ruta a la carpeta maestra del espacio de trabajo")
	inboxFlag := fs.Bool("inbox", false, "Listar las propuestas pendientes en el buzón transitorio en lugar de las skills activas")

	if err := fs.Parse(args); err != nil {
		fmt.Printf("Error al analizar banderas: %v\n", err)
		os.Exit(1)
	}

	baseDir, err := filepath.Abs(*pathFlag)
	if err != nil {
		fmt.Printf("Error obteniendo ruta absoluta: %v\n", err)
		os.Exit(1)
	}

	if *inboxFlag {
		manager := autoskill.NewManager(baseDir, nil, nil, nil)
		inbox, err := manager.ListInbox()
		if err != nil {
			fmt.Printf("Error leyendo buzón: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("================================================================================")
		fmt.Println("            Axiom — Buzón de Propuestas Transitorias (.axiom/skills/inbox/)")
		fmt.Println("================================================================================")
		if len(inbox) == 0 {
			fmt.Println("El buzón está vacío. Ejecuta 'axiom skill scan' para detectar directrices.")
			fmt.Println("================================================================================")
			return
		}

		fmt.Printf("%-28s %-10s %-20s %-12s %s\n", "NOMBRE", "ORIGEN", "INTEGRIDAD", "ROL", "JUSTIFICACIÓN")
		fmt.Println("--------------------------------------------------------------------------------")
		for _, p := range inbox {
			ver := "OK (SHA-256)"
			if !p.Metadata.Verified {
				ver = "NO VERIFICADO"
			}
			role := p.Metadata.Role
			if role == "" {
				role = "core"
			}
			fmt.Printf("%-28s %-10s %-20s %-12s %s\n",
				p.Metadata.Name, p.Metadata.Origin, ver, role, p.Metadata.Justification)
		}
		fmt.Println("--------------------------------------------------------------------------------")
		fmt.Println("Usa 'axiom skill approve <nombre>' para instalar en skills/ o 'axiom skill reject <nombre>' para descartar.")
		fmt.Println("================================================================================")
	} else {
		svc := dashboard.NewService(baseDir)
		skills, err := svc.GetSkills()
		if err != nil {
			fmt.Printf("Error listando skills: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("================================================================================")
		fmt.Println("                  Axiom — Catálogo de Skills Activas (skills/)")
		fmt.Println("================================================================================")
		if len(skills) == 0 {
			fmt.Println("No hay skills instaladas en skills/.")
			fmt.Println("================================================================================")
			return
		}

		fmt.Printf("%-32s %-30s %s\n", "NOMBRE", "RUTA", "DESCRIPCIÓN")
		fmt.Println("--------------------------------------------------------------------------------")
		for _, s := range skills {
			desc := s.Description
			if len(desc) > 40 {
				desc = desc[:37] + "..."
			}
			fmt.Printf("%-32s %-30s %s\n", s.Name, s.Path, desc)
		}
		fmt.Println("================================================================================")
	}
}

func runSkillApprove(args []string) {
	if len(args) < 1 || strings.HasPrefix(args[0], "-") {
		fmt.Println("Uso: axiom skill approve <nombre-de-skill> [--path <directorio>]")
		os.Exit(1)
	}

	skillName := args[0]
	fs := flag.NewFlagSet("skill approve", flag.ExitOnError)
	pathFlag := fs.String("path", ".", "Ruta a la carpeta maestra del espacio de trabajo")
	if err := fs.Parse(args[1:]); err != nil {
		fmt.Printf("Error al analizar banderas: %v\n", err)
		os.Exit(1)
	}

	baseDir, err := filepath.Abs(*pathFlag)
	if err != nil {
		fmt.Printf("Error resolviendo ruta: %v\n", err)
		os.Exit(1)
	}

	manager := autoskill.NewManager(baseDir, nil, nil, nil)
	if err := manager.Approve(skillName); err != nil {
		fmt.Printf("Error al aprobar skill '%s': %v\n", skillName, err)
		os.Exit(1)
	}

	fmt.Printf("\n✓ Éxito: Skill '%s' aprobada e instalada en skills/%s/SKILL.md\n", skillName, skillName)
	fmt.Println("La directriz queda disponible de inmediato para todos los agentes y desarrolladores de Axiom.")
}

func runSkillReject(args []string) {
	if len(args) < 1 || strings.HasPrefix(args[0], "-") {
		fmt.Println("Uso: axiom skill reject <nombre-de-skill> [--path <directorio>]")
		os.Exit(1)
	}

	skillName := args[0]
	fs := flag.NewFlagSet("skill reject", flag.ExitOnError)
	pathFlag := fs.String("path", ".", "Ruta a la carpeta maestra del espacio de trabajo")
	if err := fs.Parse(args[1:]); err != nil {
		fmt.Printf("Error al analizar banderas: %v\n", err)
		os.Exit(1)
	}

	baseDir, err := filepath.Abs(*pathFlag)
	if err != nil {
		fmt.Printf("Error resolviendo ruta: %v\n", err)
		os.Exit(1)
	}

	manager := autoskill.NewManager(baseDir, nil, nil, nil)
	if err := manager.Reject(skillName); err != nil {
		fmt.Printf("Error al descartar propuesta '%s': %v\n", skillName, err)
		os.Exit(1)
	}

	fmt.Printf("\n✓ Éxito: Propuesta '%s' descartada y purgada del buzón transitorio.\n\n", skillName)
}

func runSemanticStatus(args []string) {
	fs := flag.NewFlagSet("semantic status", flag.ExitOnError)
	pathFlag := fs.String("path", ".", "Ruta a la carpeta raíz del workspace")
	_ = fs.Parse(args)

	baseDir, err := filepath.Abs(*pathFlag)
	if err != nil {
		fmt.Printf("Error resolviendo ruta: %v\n", err)
		os.Exit(1)
	}

	svc := semantic.NewService(baseDir, nil, nil)
	status, err := svc.GetStatus(context.Background())
	if err != nil {
		fmt.Printf("[ERROR] Error evaluando estado semántico: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nAxiom Semantic Code Engine — Diagnóstico de Entorno\n")
	fmt.Printf("===================================================\n")
	fmt.Printf("  - Conector activo:      %s\n", strings.ToUpper(string(status.ActiveConnector)))
	fmt.Printf("  - Conector configurado: %s\n", status.ConfiguredConnector)
	fmt.Printf("  - Serena MCP:           %s\n", boolToStatus(status.SerenaAvailable))
	fmt.Printf("  - CodeGraph CLI:        %s\n", boolToStatus(status.CodeGraphAvailable))
	fmt.Printf("  - Motor Nativo Go AST:  LISTO [OK]\n")
	fmt.Printf("  - Total paquetes:       %d\n", status.TotalPackages)
	fmt.Printf("  - Total símbolos:       %d\n", status.TotalSymbols)

	if len(status.Agents) > 0 {
		fmt.Printf("\nAgentes de Desarrollo Verificados:\n")
		for _, ag := range status.Agents {
			statusBadge := "[NO DETECTADO]"
			if ag.Configured {
				statusBadge = "[CONFIGURADO]"
			}
			fmt.Printf("  * %-25s %-15s %s\n", ag.AgentName, statusBadge, ag.Details)
		}
	}

	if len(status.Warnings) > 0 {
		fmt.Printf("\nAdvertencias:\n")
		for i, w := range status.Warnings {
			fmt.Printf("  [%d] %s\n", i+1, w)
		}
	}
	fmt.Println()
}

func runSemanticSymbols(args []string) {
	fs := flag.NewFlagSet("semantic symbols", flag.ExitOnError)
	queryFlag := fs.String("query", "", "Texto a buscar en el nombre o signatura del símbolo")
	kindFlag := fs.String("kind", "", "Filtrar por tipo de símbolo: struct, interface, func, method")
	roleFlag := fs.String("role", "", "Filtrar por rol del workspace")
	pathFlag := fs.String("path", ".", "Ruta a la carpeta raíz del workspace")
	_ = fs.Parse(args)

	baseDir, err := filepath.Abs(*pathFlag)
	if err != nil {
		fmt.Printf("Error resolviendo ruta: %v\n", err)
		os.Exit(1)
	}

	svc := semantic.NewService(baseDir, nil, nil)
	symbols, err := svc.FindSymbols(semantic.SemanticQuery{
		Query: *queryFlag,
		Kind:  semantic.SymbolKind(*kindFlag),
		Role:  *roleFlag,
	})
	if err != nil {
		fmt.Printf("[ERROR] Error consultando símbolos: %v\n", err)
		os.Exit(1)
	}

	if len(symbols) == 0 {
		fmt.Println("\nNo se encontraron símbolos coincidentes.")
		return
	}

	fmt.Printf("\nCatálogo de Símbolos Semánticos (%d encontrados):\n", len(symbols))
	fmt.Printf("%-32s %-12s %-20s %s\n", "SÍMBOLO", "TIPO", "PAQUETE", "UBICACIÓN")
	fmt.Println(strings.Repeat("-", 95))
	for _, s := range symbols {
		loc := fmt.Sprintf("%s:%d", s.FilePath, s.LineNumber)
		fmt.Printf("%-32s %-12s %-20s %s\n", s.Name, s.Kind, s.Package, loc)
		if s.Signature != "" && s.Kind != semantic.KindStruct && s.Kind != semantic.KindInterface {
			fmt.Printf("   ↳ %s\n", s.Signature)
		}
	}
	fmt.Println()
}

func runSemanticInspect(args []string) {
	fs := flag.NewFlagSet("semantic inspect", flag.ExitOnError)
	roleFlag := fs.String("role", "", "Filtrar por rol del workspace")
	pathFlag := fs.String("path", ".", "Ruta a la carpeta raíz del workspace")
	_ = fs.Parse(args)

	baseDir, err := filepath.Abs(*pathFlag)
	if err != nil {
		fmt.Printf("Error resolviendo ruta: %v\n", err)
		os.Exit(1)
	}

	svc := semantic.NewService(baseDir, nil, nil)
	deps, err := svc.InspectDependencies(*roleFlag)
	if err != nil {
		fmt.Printf("[ERROR] Error analizando dependencias: %v\n", err)
		os.Exit(1)
	}

	if len(deps) == 0 {
		fmt.Println("\nNo se registraron dependencias de paquetes.")
		return
	}

	fmt.Printf("\nGrafo de Dependencias entre Paquetes (%d relaciones):\n", len(deps))
	fmt.Printf("%-25s %-5s %-45s %s\n", "ORIGEN", "", "DESTINO", "TIPO")
	fmt.Println(strings.Repeat("-", 90))
	for _, d := range deps {
		typ := "EXTERNO / STD"
		if d.IsInternal {
			typ = "INTERNO (Axiom)"
		}
		fmt.Printf("%-25s ➔    %-45s %s\n", d.SourcePackage, d.TargetPackage, typ)
	}
	fmt.Println()
}

func boolToStatus(b bool) string {
	if b {
		return "DISPONIBLE [OK]"
	}
	return "NO DETECTADO"
}

func runArchiveSync(args []string) {
	fs := flag.NewFlagSet("archive sync", flag.ExitOnError)
	pathFlag := fs.String("path", ".", "Ruta a la carpeta raíz del workspace")
	_ = fs.Parse(args)

	baseDir, err := filepath.Abs(*pathFlag)
	if err != nil {
		fmt.Printf("Error resolviendo ruta: %v\n", err)
		os.Exit(1)
	}

	svc := livingdoc.NewService(baseDir, nil, nil)
	report, err := svc.Sync(context.Background())
	if err != nil {
		fmt.Printf("[ERROR] Fallo al sincronizar especificaciones vivas: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[OK] Sincronización de Especificaciones Vivas completada con éxito.\n")
	fmt.Printf("  - Total de especificaciones consolidadas: %d\n", report.SpecsCount)
	fmt.Printf("  - Total de requisitos activos: %d\n", report.RequirementsCount)
	fmt.Printf("  - Total de escenarios BDD: %d\n", report.ScenariosCount)
	fmt.Printf("  - Catálogo maestro regenerado: %s\n\n", report.IndexPath)
}

func runArchiveList(args []string) {
	fs := flag.NewFlagSet("archive list", flag.ExitOnError)
	pathFlag := fs.String("path", ".", "Ruta a la carpeta raíz del workspace")
	_ = fs.Parse(args)

	baseDir, err := filepath.Abs(*pathFlag)
	if err != nil {
		fmt.Printf("Error resolviendo ruta: %v\n", err)
		os.Exit(1)
	}

	svc := livingdoc.NewService(baseDir, nil, nil)
	catalog, err := svc.GetCatalog(context.Background())
	if err != nil {
		fmt.Printf("[ERROR] Fallo al obtener catálogo de especificaciones vivas: %v\n", err)
		os.Exit(1)
	}

	if len(catalog.Specs) == 0 {
		fmt.Println("\nNo se encontraron especificaciones vivas en openspec/specs/.")
		fmt.Println("Ejecuta 'axiom archive sync' o 'axiom archive coldstart' para generarlas.")
		return
	}

	fmt.Printf("\nCatálogo Maestro de Especificaciones Vivas (%d dominios, %d requisitos totales):\n", len(catalog.Specs), catalog.TotalRequirements)
	fmt.Printf("%-25s %-12s %-12s %s\n", "DOMINIO", "REQUISITOS", "ESCENARIOS", "TÍTULO")
	fmt.Println(strings.Repeat("-", 75))
	for _, d := range catalog.Specs {
		fmt.Printf("%-25s %-12d %-12d %s\n", d.Domain, len(d.Requirements), d.TotalScenarios, d.Title)
	}
	fmt.Println()
}

func runArchiveShow(args []string) {
	fs := flag.NewFlagSet("archive show", flag.ExitOnError)
	domainFlag := fs.String("domain", "", "Dominio de la especificación viva (ej. living-documentation, multi-role-governance)")
	pathFlag := fs.String("path", ".", "Ruta a la carpeta raíz del workspace")
	_ = fs.Parse(args)

	domain := *domainFlag
	if domain == "" && len(fs.Args()) > 0 {
		domain = fs.Args()[0]
	}

	if domain == "" {
		fmt.Println("[ERROR] Debes especificar un dominio con --domain <nombre> o como argumento posicional.")
		os.Exit(1)
	}

	baseDir, err := filepath.Abs(*pathFlag)
	if err != nil {
		fmt.Printf("Error resolviendo ruta: %v\n", err)
		os.Exit(1)
	}

	svc := livingdoc.NewService(baseDir, nil, nil)
	entry, content, err := svc.GetSpecDetail(domain)
	if err != nil {
		fmt.Printf("[ERROR] %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n=== ESPECIFICACIÓN VIVA: %s ===\n", entry.Title)
	fmt.Printf("Dominio: %s | Requisitos Activos: %d | Escenarios: %d | Modificado: %s\n\n", entry.Domain, len(entry.Requirements), entry.TotalScenarios, entry.LastModified.Format("2006-01-02 15:04:05"))
	fmt.Println(content)
}

func runArchiveColdStart(args []string) {
	fs := flag.NewFlagSet("archive coldstart", flag.ExitOnError)
	changeFlag := fs.String("change", "", "Nombre del cambio archivado a promover (ej. 2026-09-15-inc-01-workspace-topology-contracts)")
	domainFlag := fs.String("domain", "", "Nombre del dominio de destino (opcional, inferido si está vacío)")
	pathFlag := fs.String("path", ".", "Ruta a la carpeta raíz del workspace")
	_ = fs.Parse(args)

	if *changeFlag == "" {
		fmt.Println("[ERROR] La bandera --change <nombre> es obligatoria para ejecutar coldstart.")
		os.Exit(1)
	}

	baseDir, err := filepath.Abs(*pathFlag)
	if err != nil {
		fmt.Printf("Error resolviendo ruta: %v\n", err)
		os.Exit(1)
	}

	svc := livingdoc.NewService(baseDir, nil, nil)
	entry, err := svc.ColdStart(*changeFlag, *domainFlag)
	if err != nil {
		fmt.Printf("[ERROR] Fallo en la síntesis de Adopción Orgánica (Cold Start): %v\n", err)
		os.Exit(1)
	}

	fmt.Println("[OK] Adopción Orgánica (Zero-Doc Cold Start) ejecutada exitosamente.")
	fmt.Printf("  - Cambio fuente: %s\n", *changeFlag)
	fmt.Printf("  - Dominio vivo: %s\n", entry.Domain)
	fmt.Printf("  - Requisitos sintetizados: %d\n", len(entry.Requirements))
	fmt.Printf("  - Escenarios BDD: %d\n", entry.TotalScenarios)
	fmt.Printf("  - Especificación viva consolidada: %s\n", entry.FilePath)
	fmt.Println("  - Catálogo maestro INDEX.md actualizado automáticamente.")
	fmt.Println()
}

func runInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	nameFlag := fs.String("name", "", "Nombre descriptivo del proyecto (por defecto: nombre del directorio)")
	pathFlag := fs.String("path", ".", "Ruta del directorio a inicializar")
	topologyFlag := fs.String("topology", "monorepo-embedded", "Topología del workspace (monorepo-embedded, multirepo)")
	forceFlag := fs.Bool("force", false, "Sobreescribir axiom.yaml si ya existe")
	_ = fs.Parse(args)

	absPath, err := filepath.Abs(*pathFlag)
	if err != nil {
		fmt.Printf("[ERROR] Ruta inválida: %v\n", err)
		os.Exit(1)
	}

	hubMgr, err := hub.NewManager("")
	if err != nil {
		fmt.Printf("[AVISO] No se pudo conectar con el registro global del hub: %v\n", err)
	}

	det := hub.NewDetector()
	ini := hub.NewInitializer(hubMgr, det)

	res, err := ini.Init(hub.InitOptions{
		Path:     absPath,
		Name:     *nameFlag,
		Topology: *topologyFlag,
		Force:    *forceFlag,
	})
	if err != nil {
		fmt.Printf("[ERROR] Fallo al inicializar el proyecto: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("================================================================================")
	if res.AlreadyExisted {
		fmt.Println("             Axiom — Proyecto Vinculado al Hub Global")
	} else {
		fmt.Println("             Axiom — Proyecto Inicializado Exitosamente")
	}
	fmt.Println("================================================================================")
	fmt.Printf("  - Proyecto:            %s (ID: %s)\n", res.Record.Name, res.Record.ID)
	fmt.Printf("  - Directorio:          %s\n", res.Record.Path)
	fmt.Printf("  - Topología:           %s\n", res.Record.Topology)
	fmt.Printf("  - Archivo Config:      %s\n", res.ConfigPath)
	if res.AlreadyExisted {
		fmt.Println("  - Estado:              Ya contenía axiom.yaml (registrado y marcado como activo)")
	} else {
		fmt.Println("  - Estructura creada:   openspec/specs/, openspec/changes/, .axiom/inbox/skills/")
	}
	if hubMgr != nil {
		fmt.Printf("  - Registro global:     %s [ACTIVO]\n", hubMgr.GetConfigPath())
	}
	fmt.Println("================================================================================")
	fmt.Println("¡Listo! Ejecuta 'axiom ui' para abrir el panel de control interactivo.")
}

func runProjectList(args []string) {
	hubMgr, err := hub.NewManager("")
	if err != nil {
		fmt.Printf("[ERROR] No se pudo cargar el gestor de Hub: %v\n", err)
		os.Exit(1)
	}

	cfg, err := hubMgr.Load()
	if err != nil {
		fmt.Printf("[ERROR] Error cargando catálogo de proyectos: %v\n", err)
		os.Exit(1)
	}

	if len(cfg.Workspaces) == 0 {
		fmt.Println("\nNo hay proyectos registrados en el Hub global de Axiom (~/.axiom/workspaces.json).")
		fmt.Println("Usa 'axiom init' o 'axiom project add <ruta>' para incorporar proyectos.")
		return
	}

	fmt.Printf("\nCatálogo Global de Proyectos de Axiom (%d registrados):\n", len(cfg.Workspaces))
	fmt.Printf("%-18s %-20s %-8s %-20s %-14s %s\n", "ID", "NOMBRE", "ACTIVO", "TOPOLOGÍA", "ESTADO", "RUTA")
	fmt.Println(strings.Repeat("-", 100))
	for _, w := range cfg.Workspaces {
		activeMark := ""
		if w.ID == cfg.ActiveWorkspace || w.Path == cfg.ActiveWorkspace {
			activeMark = "★ SÍ"
		}
		status := "CONFIGURADO"
		if !fileExists(filepath.Join(w.Path, "axiom.yaml")) {
			status = "SIN CONFIG"
		}
		fmt.Printf("%-18s %-20s %-8s %-20s %-14s %s\n", w.ID, w.Name, activeMark, w.Topology, status, w.Path)
	}
	fmt.Println()
}

func runProjectSwitch(args []string) {
	if len(args) == 0 {
		fmt.Println("Error: debes especificar el ID, nombre o ruta del proyecto. Ej: 'axiom project switch ludeka'")
		os.Exit(1)
	}
	target := args[0]

	hubMgr, err := hub.NewManager("")
	if err != nil {
		fmt.Printf("[ERROR] No se pudo cargar el gestor de Hub: %v\n", err)
		os.Exit(1)
	}

	rec, err := hubMgr.SetActive(target)
	if err != nil {
		fmt.Printf("[ERROR] No se pudo conmutar el proyecto: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[OK] Proyecto activo conmutado exitosamente:\n")
	fmt.Printf("  - ID:     %s\n", rec.ID)
	fmt.Printf("  - Nombre: %s\n", rec.Name)
	fmt.Printf("  - Ruta:   %s\n", rec.Path)
}

func runProjectAdd(args []string) {
	fs := flag.NewFlagSet("project add", flag.ExitOnError)
	nameFlag := fs.String("name", "", "Nombre descriptivo del proyecto")
	topologyFlag := fs.String("topology", "monorepo-embedded", "Topología del proyecto")
	_ = fs.Parse(args)

	if len(fs.Args()) == 0 {
		fmt.Println("Error: debes especificar la ruta del directorio. Ej: 'axiom project add C:\\repos\\ludeka'")
		os.Exit(1)
	}
	targetPath := fs.Args()[0]

	hubMgr, err := hub.NewManager("")
	if err != nil {
		fmt.Printf("[ERROR] No se pudo cargar el gestor de Hub: %v\n", err)
		os.Exit(1)
	}

	rec, err := hubMgr.Register(targetPath, *nameFlag, *topologyFlag)
	if err != nil {
		fmt.Printf("[ERROR] Error registrando proyecto: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[OK] Proyecto registrado en el Hub global:\n")
	fmt.Printf("  - ID:        %s\n", rec.ID)
	fmt.Printf("  - Nombre:    %s\n", rec.Name)
	fmt.Printf("  - Ruta:      %s\n", rec.Path)
	fmt.Printf("  - Topología: %s\n", rec.Topology)
}

func runProjectRemove(args []string) {
	if len(args) == 0 {
		fmt.Println("Error: debes especificar el ID o ruta del proyecto a desvincular. Ej: 'axiom project remove ludeka'")
		os.Exit(1)
	}
	target := args[0]

	hubMgr, err := hub.NewManager("")
	if err != nil {
		fmt.Printf("[ERROR] No se pudo cargar el gestor de Hub: %v\n", err)
		os.Exit(1)
	}

	if err := hubMgr.Unregister(target); err != nil {
		fmt.Printf("[ERROR] Error al desvincular proyecto: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[OK] Proyecto '%s' desvinculado del Hub global (los archivos en disco no fueron alterados).\n", target)
}

func runSDD(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "Uso: axiom sdd <subcomando> [argumentos]")
		fmt.Fprintln(stdout, "\nSubcomandos disponibles:")
		fmt.Fprintln(stdout, "  status           Consulta el estado de fases y artefactos de un cambio SDD (--json, --instructions)")
		fmt.Fprintln(stdout, "  continue         Calcula y emite la siguiente acción autorizada del despachador SDD")
		fmt.Fprintln(stdout, "  attempt          Gestiona el presupuesto y libro mayor de intentos de ejecución (acquire/settle)")
		fmt.Fprintln(stdout, "  verify-validate  Valida un reporte de verificación contra las especificaciones activas")
		fmt.Fprintln(stdout, "  archive-compose  Compone el reporte de archivado formal y actualiza las especificaciones vivas")
		fmt.Fprintln(stdout, "  task-result      Valida y extrae el resultado tipado de una fase delegada")
		fmt.Fprintln(stdout, "  preflight-hook   Ejecuta el hook previo de verificación SDD")
		if len(args) < 1 {
			return 1
		}
		return 0
	}

	subCmd := args[0]
	subArgs := args[1:]
	var err error

	switch subCmd {
	case "status":
		err = cli.RunSDDStatus(subArgs, stdout)
	case "continue":
		err = cli.RunSDDContinue(subArgs, stdout)
	case "attempt":
		err = cli.RunSDDAttempt(cli.CanonicalizeSDDAttemptRevisionArgs(subArgs), stdout)
	case "verify-validate":
		err = cli.RunSDDVerifyValidate(subArgs, stdout)
	case "archive-compose":
		err = cli.RunSDDArchiveCompose(subArgs, stdout)
	case "task-result":
		err = cli.RunSDDTaskResult(subArgs, stdout)
	case "preflight-hook":
		err = cli.RunSDDPreflightHook(subArgs, stdout)
	default:
		fmt.Fprintf(stderr, "Error: subcomando '%s' no reconocido para sdd. Opciones: status, continue, attempt, verify-validate, archive-compose, task-result, preflight-hook\n", subCmd)
		return 1
	}

	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

// runODD despacha los subcomandos de "axiom odd" (REQ-19.5, REQ-19.6,
// REQ-19.7, REQ-19.8): create, status y promote. Calcado de runSDD: ayuda
// con exit 1 cuando falta el subcomando, exit 0 con --help/-h, mensaje
// explícito y exit 1 ante un subcomando desconocido.
func runODD(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "Uso: axiom odd <subcomando> [argumentos]")
		fmt.Fprintln(stdout, "\nSubcomandos disponibles:")
		fmt.Fprintln(stdout, "  create           Crea un documento vivo ODD para una nueva feature (--cwd)")
		fmt.Fprintln(stdout, "  status           Consulta el progreso de los documentos vivos ODD (--json, --check-mirror, --cwd)")
		fmt.Fprintln(stdout, "  promote          Promueve un documento vivo ODD a una propuesta SDD sembrada (--name, --intent, --type, --dry-run, --cwd)")
		if len(args) < 1 {
			return 1
		}
		return 0
	}

	subCmd := args[0]
	subArgs := args[1:]
	var err error

	switch subCmd {
	case "create":
		err = cli.RunODDCreate(subArgs, stdout)
	case "status":
		err = cli.RunODDStatus(subArgs, stdout)
	case "promote":
		err = cli.RunODDPromote(subArgs, stdout, newDashboardScaffolder)
	default:
		fmt.Fprintf(stderr, "Error: subcomando '%s' no reconocido para odd. Opciones: create, status, promote\n", subCmd)
		return 1
	}

	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

// dashboardScaffolder satisface odd.Scaffolder invocando
// dashboard.Service.CreateIncrement bajo root para "axiom odd promote"
// [D-01, D-07]. Vive en package main -- no en internal/cli, como planteaba
// el diseño (§5.5, §5.8) -- porque internal/dashboard/service.go:19 ya
// importa internal/cli (para RunSDDContinue, RunSDDVerifyValidate,
// RunDoctor y RunRestore): si internal/cli importara internal/dashboard
// para construir este adaptador, el grafo cerraría el ciclo
// cli → dashboard → cli, con independencia total de internal/odd, que
// sigue siendo una hoja del árbol de dependencias sin este cambio.
// package main no lo importa nadie, así que es el único punto del árbol de
// dependencias actual que puede importar ambos paquetes y construir el
// adaptador sin ciclar [desviación deliberada de D-01, documentada en el
// informe de esta fase].
type dashboardScaffolder struct {
	root string
}

// newDashboardScaffolder construye el dashboardScaffolder de producción
// para root. Es el único valor que runODD pasa a cli.RunODDPromote; los
// tests de internal/cli pasan en su lugar un Scaffolder falso.
func newDashboardScaffolder(root string) odd.Scaffolder {
	return dashboardScaffolder{root: root}
}

// Scaffold traduce req al contrato de dashboard.CreateIncrementRequest y
// proyecta la respuesta a odd.ScaffoldResult. El error de
// dashboard.Service.CreateIncrement se propaga sin envolver: su mensaje
// (colisión con un cambio activo o archivado, nombre inválido) ya es la
// explicación completa que el usuario de "axiom odd promote" debe leer,
// idéntica a la que produce "axiom change create" para el mismo fallo
// (REQ-19.10).
func (a dashboardScaffolder) Scaffold(req odd.ScaffoldRequest) (odd.ScaffoldResult, error) {
	resp, err := dashboard.NewService(a.root).CreateIncrement(dashboard.CreateIncrementRequest{
		Name:         req.Name,
		Intent:       req.Intent,
		Type:         req.Type,
		ProposalBody: req.ProposalBody,
	})
	if err != nil {
		return odd.ScaffoldResult{}, err
	}
	return odd.ScaffoldResult{Name: resp.Name, Path: resp.Path}, nil
}

// runSimpleCommand adapts a plain `func([]string, io.Writer) error` handler to
// this dispatcher's exit-code convention, reporting the error on stderr the way
// every other branch does.
func runSimpleCommand(handler func([]string, io.Writer) error, args []string, stdout, stderr io.Writer) int {
	if err := handler(args, stdout); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

// runReview serves both `axiom review <subcommand>` and the flat
// `axiom review-<verb>` aliases. flatAlias tells them apart: only the aliases
// reach the standalone handlers; the subcommand form goes to the facade.
func runReview(args []string, stdout, stderr io.Writer, flatAlias bool) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(stdout, "Uso: axiom review <subcomando> [argumentos]")
		fmt.Fprintln(stdout, "\nSubcomandos disponibles:")
		fmt.Fprintln(stdout, "  mode             Consulta o modifica el estado de RDD (enable, disable, status)")
		fmt.Fprintln(stdout, "  start            Inicia formalmente una revisión sobre el candidato actual")
		fmt.Fprintln(stdout, "  resume           Reanuda una transacción de revisión pendiente")
		fmt.Fprintln(stdout, "  step             Avanza al siguiente paso de revisión")
		fmt.Fprintln(stdout, "  bundle-export    Exporta un paquete de evidencia de revisión")
		fmt.Fprintln(stdout, "  bundle-import    Importa un paquete de evidencia de revisión")
		fmt.Fprintln(stdout, "  validate         Ejecuta validaciones no decisivas sobre el candidato")
		return 0
	}

	if len(args) >= 1 && args[0] == "mode" {
		if err := cli.RunReviewMode(args[1:], stdout); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		return 0
	}
	// Only the flat aliases (`axiom review-start`, `review-step`, …) map to the
	// standalone handlers below. `axiom review <sub>` belongs to the facade,
	// exactly as internal/app/app.go dispatches it: routing `review start`
	// to cli.RunReviewStart bound the v2 atomic lifecycle to the legacy flat
	// command, so a START handed back by `review status --next-transition`
	// was rejected for a missing --policy-file it never names.
	if flatAlias && len(args) >= 1 {
		switch args[0] {
		case "start":
			if err := cli.RunReviewStart(args[1:], stdout); err != nil {
				fmt.Fprintf(stderr, "Error: %v\n", err)
				return 1
			}
			return 0
		case "resume":
			if err := cli.RunReviewResume(args[1:], stdout); err != nil {
				fmt.Fprintf(stderr, "Error: %v\n", err)
				return 1
			}
			return 0
		case "step":
			if err := cli.RunReviewStep(args[1:], stdout); err != nil {
				fmt.Fprintf(stderr, "Error: %v\n", err)
				return 1
			}
			return 0
		case "bundle-export":
			if err := cli.RunReviewBundleExport(args[1:], stdout); err != nil {
				fmt.Fprintf(stderr, "Error: %v\n", err)
				return 1
			}
			return 0
		case "bundle-import":
			if err := cli.RunReviewBundleImport(args[1:], stdout); err != nil {
				fmt.Fprintf(stderr, "Error: %v\n", err)
				return 1
			}
			return 0
		case "validate":
			if err := cli.RunReviewValidateNonDeciding(args[1:], stdout); err != nil {
				fmt.Fprintf(stderr, "Error: %v\n", err)
				return 1
			}
			return 0
		}
	}
	if err := cli.RunReview(args, stdout); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

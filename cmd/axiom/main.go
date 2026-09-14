package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/handoff"
	"github.com/gentleman-programming/gentle-ai/v2/internal/workspace"
)

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

COMANDOS:
  workspace validate   Valida la configuración de axiom.yaml y la topología de repositorios
  handoff show         Muestra el relevo activo de un cambio
  handoff create       Genera una plantilla canónica de relevo (handoff.md)
  handoff validate     Valida la consistencia semántica y transición de fases de un relevo
  version              Muestra la versión e información de compilación
  help                 Muestra esta ayuda

BANDERAS:
  --version, -v        Muestra la versión de Axiom
  --help, -h           Muestra esta ayuda

Ejemplos:
  axiom workspace validate
  axiom handoff show --change inc-02-structured-handoffs-lifecycle
  axiom handoff create --change mi-cambio --from spec --to design --from-role tech-lead --to-role architect
  axiom handoff validate --change inc-02-structured-handoffs-lifecycle
`
	fmt.Print(help)
}

func printVersion() {
	fmt.Printf("%s version %s (%s/%s) commit:%s\n", Platform, Version, runtime.GOOS, runtime.GOARCH, GitCommit)
}

func main() {
	if len(os.Args) < 2 {
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

	default:
		fmt.Printf("Error: comando '%s' no reconocido.\n\n", arg1)
		printHelp()
		os.Exit(1)
	}
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

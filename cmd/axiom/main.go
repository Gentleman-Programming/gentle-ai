package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

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
  axiom <comando> [argumentos]
  axiom [banderas]

COMANDOS:
  workspace validate   Valida la configuración de axiom.yaml y la topología de repositorios
  version              Muestra la versión e información de compilación
  help                 Muestra esta ayuda

BANDERAS:
  --version, -v        Muestra la versión de Axiom
  --help, -h           Muestra esta ayuda

Ejemplo de validación de espacio de trabajo:
  axiom workspace validate
  axiom workspace validate --path C:\mi-proyecto
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

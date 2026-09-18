package odd

import "testing"

// TestParseCreateArgs cubre la frontera T-2 de la matriz de amenazas del
// diseño (selección de raíz de workspace vía --cwd) para "axiom odd create
// <nombre>": bandera conocida y desconocida, posicional ausente o repetido,
// y los tres casos de --cwd (ausente, relativo, absoluto).
func TestParseCreateArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    CreateArgs
		wantErr bool
	}{
		{
			name: "solo el nombre posicional usa cwd por defecto",
			args: []string{"gestion-inventario"},
			want: CreateArgs{Name: "gestion-inventario", CWD: "."},
		},
		{
			name: "cwd relativo explícito",
			args: []string{"gestion-inventario", "--cwd", "./workspace"},
			want: CreateArgs{Name: "gestion-inventario", CWD: "./workspace"},
		},
		{
			name: "cwd absoluto explícito",
			args: []string{"gestion-inventario", "--cwd", `C:\repos\demo`},
			want: CreateArgs{Name: "gestion-inventario", CWD: `C:\repos\demo`},
		},
		{
			name:    "posicional ausente",
			args:    []string{"--cwd", "."},
			wantErr: true,
		},
		{
			name:    "bandera desconocida",
			args:    []string{"gestion-inventario", "--json"},
			wantErr: true,
		},
		{
			name:    "--cwd sin valor",
			args:    []string{"gestion-inventario", "--cwd"},
			wantErr: true,
		},
		{
			name:    "segundo posicional inesperado",
			args:    []string{"gestion-inventario", "otro-nombre"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCreateArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseCreateArgs(%v) = %+v, se esperaba error", tt.args, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseCreateArgs(%v) devolvió error inesperado: %v", tt.args, err)
			}
			if got != tt.want {
				t.Errorf("ParseCreateArgs(%v) = %+v, se esperaba %+v", tt.args, got, tt.want)
			}
		})
	}
}

// TestParseStatusArgs cubre "axiom odd status [feature]": a diferencia de
// create y promote, el positional feature es opcional (REQ-19.6), y --json
// puede combinarse con --check-mirror (REQ-19.7) sin conflicto.
func TestParseStatusArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    StatusArgs
		wantErr bool
	}{
		{
			name: "sin argumentos usa valores por defecto",
			args: []string{},
			want: StatusArgs{CWD: "."},
		},
		{
			name: "feature posicional opcional",
			args: []string{"gestion-inventario"},
			want: StatusArgs{Feature: "gestion-inventario", CWD: "."},
		},
		{
			name: "--json activa la salida estructurada",
			args: []string{"--json"},
			want: StatusArgs{CWD: ".", JSON: true},
		},
		{
			name: "--json combinado con --check-mirror",
			args: []string{"--json", "--check-mirror"},
			want: StatusArgs{CWD: ".", JSON: true, CheckMirror: true},
		},
		{
			name: "cwd relativo explícito",
			args: []string{"--cwd", "../otro"},
			want: StatusArgs{CWD: "../otro"},
		},
		{
			name: "cwd absoluto explícito",
			args: []string{"--cwd", `C:\repos\demo`},
			want: StatusArgs{CWD: `C:\repos\demo`},
		},
		{
			name:    "bandera desconocida",
			args:    []string{"--bogus"},
			wantErr: true,
		},
		{
			name:    "--cwd sin valor",
			args:    []string{"--cwd"},
			wantErr: true,
		},
		{
			name:    "segundo posicional inesperado",
			args:    []string{"uno", "dos"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseStatusArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseStatusArgs(%v) = %+v, se esperaba error", tt.args, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseStatusArgs(%v) devolvió error inesperado: %v", tt.args, err)
			}
			if got != tt.want {
				t.Errorf("ParseStatusArgs(%v) = %+v, se esperaba %+v", tt.args, got, tt.want)
			}
		})
	}
}

// TestParsePromoteArgs cubre "axiom odd promote <feature>": las banderas
// --name, --dry-run, --intent y --type (su consumo en la CLI llega en la
// Fase 5; el parseo se cubre en esta rebanada), más el positional feature
// obligatorio y los tres casos de --cwd.
func TestParsePromoteArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    PromoteArgs
		wantErr bool
	}{
		{
			name: "solo el feature posicional usa cwd por defecto",
			args: []string{"gestion-inventario"},
			want: PromoteArgs{Feature: "gestion-inventario", CWD: "."},
		},
		{
			name: "--name sobrescribe el nombre de cambio derivado",
			args: []string{"gestion-inventario", "--name", "modulo-inventario-v2"},
			want: PromoteArgs{Feature: "gestion-inventario", CWD: ".", Name: "modulo-inventario-v2"},
		},
		{
			name: "--dry-run activa la previsualización",
			args: []string{"gestion-inventario", "--dry-run"},
			want: PromoteArgs{Feature: "gestion-inventario", CWD: ".", DryRun: true},
		},
		{
			name: "--intent y --type combinados",
			args: []string{"gestion-inventario", "--intent", "Propósito X", "--type", "feature"},
			want: PromoteArgs{Feature: "gestion-inventario", CWD: ".", Intent: "Propósito X", Type: "feature"},
		},
		{
			name: "cwd relativo explícito",
			args: []string{"gestion-inventario", "--cwd", "./workspace"},
			want: PromoteArgs{Feature: "gestion-inventario", CWD: "./workspace"},
		},
		{
			name: "cwd absoluto explícito",
			args: []string{"gestion-inventario", "--cwd", `C:\repos\demo`},
			want: PromoteArgs{Feature: "gestion-inventario", CWD: `C:\repos\demo`},
		},
		{
			name:    "posicional ausente",
			args:    []string{"--dry-run"},
			wantErr: true,
		},
		{
			name:    "bandera desconocida",
			args:    []string{"gestion-inventario", "--bogus"},
			wantErr: true,
		},
		{
			name:    "--name sin valor",
			args:    []string{"gestion-inventario", "--name"},
			wantErr: true,
		},
		{
			name:    "segundo posicional inesperado",
			args:    []string{"gestion-inventario", "otro"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePromoteArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParsePromoteArgs(%v) = %+v, se esperaba error", tt.args, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePromoteArgs(%v) devolvió error inesperado: %v", tt.args, err)
			}
			if got != tt.want {
				t.Errorf("ParsePromoteArgs(%v) = %+v, se esperaba %+v", tt.args, got, tt.want)
			}
		})
	}
}

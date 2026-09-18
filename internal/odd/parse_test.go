package odd

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// sampleActiveDocument es un documento vivo activo que deliberadamente
// omite la sección canónica "Restricciones" y contiene, en su checklist,
// una tarea sin identificador estable. Sirve de fixture compartida para
// varios casos de esta batería: ambas irregularidades deben degradarse a
// Warnings, nunca a error.
const sampleActiveDocument = `# ODD: gestion-inventario

> **Estado:** activo

_Documento creado el 2026-09-17._

## Objetivo

Gestionar el inventario de forma centralizada.

## Problema

No hay visibilidad del stock en tiempo real.

## Porqué

El negocio pierde ventas por rupturas de stock no detectadas.

## Alcance

Dentro: consulta y ajuste manual de stock.

## Alcance autorizado

Lectura y escritura sobre el módulo de inventario.

## Checklist accionable

- [x] [T1] Modelar el dominio de inventario
- [ ] [T2] Exponer el endpoint de consulta
- [ ] Tarea sin identificador estable

## Criterios de aceptación

- [ ] El endpoint responde en menos de 200ms

## Comprobaciones aplicables

- [ ] go test ./internal/inventario/...

## Progreso

Pendiente de calcular.

## Evidencia de verificación

_(pendiente de completar)_

## Siguiente paso

Implementar el endpoint de ajuste de stock.
`

// containsWarningAbout confirma que al menos una advertencia contiene la
// subcadena indicada, sin acoplar el test al texto exacto del mensaje.
func containsWarningAbout(warnings []string, substr string) bool {
	for _, w := range warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}

// TestParse_CabeceraDeEstado cubre la cabecera activa y la promovida
// [D-06]: Status, PromotedTo y PromotedAt deben reflejar exactamente lo que
// declara el bloque de cita en castellano peninsular.
func TestParse_CabeceraDeEstado(t *testing.T) {
	tests := []struct {
		name           string
		raw            string
		wantStatus     Status
		wantPromotedTo string
		wantPromotedAt string
	}{
		{
			name:       "cabecera activa sin marca de promoción",
			raw:        "# ODD: gestion-inventario\n\n> **Estado:** activo\n\n## Objetivo\n\nContenido.\n",
			wantStatus: StatusActive,
		},
		{
			name:           "cabecera promovida con referencia y fecha",
			raw:            "# ODD: gestion-inventario\n\n> **Estado:** promovido\n> **Promovido a:** `openspec/changes/gestion-inventario/`\n> **Promovido el:** 2026-09-17\n\n## Objetivo\n\nContenido.\n",
			wantStatus:     StatusPromoted,
			wantPromotedTo: "openspec/changes/gestion-inventario/",
			wantPromotedAt: "2026-09-17",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := Parse(tt.raw)
			if err != nil {
				t.Fatalf("Parse() devolvió error inesperado: %v", err)
			}
			if doc.Feature != "gestion-inventario" {
				t.Errorf("Feature = %q, se esperaba %q", doc.Feature, "gestion-inventario")
			}
			if doc.Status != tt.wantStatus {
				t.Errorf("Status = %q, se esperaba %q", doc.Status, tt.wantStatus)
			}
			if doc.PromotedTo != tt.wantPromotedTo {
				t.Errorf("PromotedTo = %q, se esperaba %q", doc.PromotedTo, tt.wantPromotedTo)
			}
			if doc.PromotedAt != tt.wantPromotedAt {
				t.Errorf("PromotedAt = %q, se esperaba %q", doc.PromotedAt, tt.wantPromotedAt)
			}
		})
	}
}

// TestParse_SeccionCanonicaAusenteProduceWarningSinError cubre REQ-19.1: una
// sección canónica ausente se degrada a Warnings; Parse nunca devuelve error
// por esta causa.
func TestParse_SeccionCanonicaAusenteProduceWarningSinError(t *testing.T) {
	doc, err := Parse(sampleActiveDocument)
	if err != nil {
		t.Fatalf("Parse() devolvió error inesperado: %v", err)
	}

	if _, ok := doc.Sections[SectionConstraints]; ok {
		t.Fatalf("la fixture de prueba no debía declarar %q", SectionConstraints)
	}
	if !containsWarningAbout(doc.Warnings, "Restricciones") {
		t.Errorf("Warnings = %v, se esperaba una advertencia sobre la sección ausente %q", doc.Warnings, "Restricciones")
	}
}

// TestParse_TareaSinIdentificadorProduceWarning cubre REQ-19.3: una línea de
// checklist sin identificador estable se conserva como tarea (con ID vacío)
// y se registra en Warnings.
func TestParse_TareaSinIdentificadorProduceWarning(t *testing.T) {
	doc, err := Parse(sampleActiveDocument)
	if err != nil {
		t.Fatalf("Parse() devolvió error inesperado: %v", err)
	}

	if !containsWarningAbout(doc.Warnings, "sin identificador") {
		t.Errorf("Warnings = %v, se esperaba una advertencia sobre la tarea sin identificador", doc.Warnings)
	}
}

// TestParse_ChecklistConservaIdentificadoresYEstado confirma que cada tarea
// del checklist conserva su identificador estable (o lo declara vacío) y su
// estado de completado, en el mismo orden en que aparece en el documento.
func TestParse_ChecklistConservaIdentificadoresYEstado(t *testing.T) {
	doc, err := Parse(sampleActiveDocument)
	if err != nil {
		t.Fatalf("Parse() devolvió error inesperado: %v", err)
	}

	want := []Task{
		{ID: "T1", Text: "Modelar el dominio de inventario", Done: true},
		{ID: "T2", Text: "Exponer el endpoint de consulta", Done: false},
		{ID: "", Text: "Tarea sin identificador estable", Done: false},
	}
	if !reflect.DeepEqual(doc.Tasks, want) {
		t.Errorf("Tasks = %+v, se esperaba %+v", doc.Tasks, want)
	}
}

// TestParse_DocumentoSinCabeceraDevuelveErrMalformedDocument cubre el único
// camino de error de Parse: un contenido que no es un documento ODD.
func TestParse_DocumentoSinCabeceraDevuelveErrMalformedDocument(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "sin cabecera en absoluto", raw: "## Objetivo\n\nTexto.\n"},
		{name: "cadena vacía", raw: ""},
		{name: "solo espacios en blanco", raw: "   \n\n  \n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.raw)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, se esperaba ErrMalformedDocument", tt.raw)
			}
			if !errors.Is(err, ErrMalformedDocument) {
				t.Errorf("Parse(%q) = %v, no envuelve ErrMalformedDocument", tt.raw, err)
			}
		})
	}
}

// TestParse_NormalizaCRLF confirma que un documento con terminadores CRLF se
// analiza de forma idéntica a uno con terminadores LF, y que Raw ya no
// conserva ningún '\r'.
func TestParse_NormalizaCRLF(t *testing.T) {
	raw := "# ODD: demo\r\n\r\n> **Estado:** activo\r\n\r\n## Objetivo\r\n\r\nContenido con CRLF.\r\n"

	doc, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse() devolvió error inesperado con CRLF: %v", err)
	}
	if doc.Feature != "demo" {
		t.Errorf("Feature = %q, se esperaba %q", doc.Feature, "demo")
	}
	if strings.Contains(doc.Raw, "\r") {
		t.Errorf("Raw conserva terminadores CRLF tras normalizar: %q", doc.Raw)
	}

	got := doc.Sections[SectionObjective]
	want := "Contenido con CRLF."
	if got != want {
		t.Errorf("Sections[SectionObjective] = %q, se esperaba %q", got, want)
	}
}

// TestParse_ProgresoSoloCuentaLaSeccionDeChecklist cubre la decisión D-04
// del diseño: multirole.CountTasks debe invocarse únicamente sobre la
// rebanada de texto de "Checklist accionable", nunca sobre el documento
// completo. Un documento con casillas también en "Criterios de aceptación"
// y en "Comprobaciones aplicables" demuestra que esas casillas ajenas no
// deben contar en el progreso.
func TestParse_ProgresoSoloCuentaLaSeccionDeChecklist(t *testing.T) {
	raw := `# ODD: gestion-inventario

> **Estado:** activo

## Objetivo

Contenido.

## Checklist accionable

- [x] [T1] Tarea completada del checklist
- [ ] [T2] Tarea pendiente del checklist

## Criterios de aceptación

- [ ] Este criterio no es una tarea del checklist
- [ ] Ni tampoco este otro

## Comprobaciones aplicables

- [x] go test ./...
- [ ] go vet ./...
- [ ] gofmt -l .
`

	doc, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse() devolvió error inesperado: %v", err)
	}

	if doc.Progress.Total != 2 {
		t.Fatalf("Progress.Total = %d, se esperaban 2 (sólo las tareas del checklist)", doc.Progress.Total)
	}
	if doc.Progress.Completed != 1 {
		t.Errorf("Progress.Completed = %d, se esperaba 1", doc.Progress.Completed)
	}
	if doc.Progress.Pending != 1 {
		t.Errorf("Progress.Pending = %d, se esperaba 1", doc.Progress.Pending)
	}
}

// TestParse_ChecklistVacioNoDivideEntreCero cubre el caso borde de la misma
// decisión D-04: un checklist sin ninguna tarea debe producir 0/0, nunca un
// pánico por división entre cero ni un porcentaje distinto de cero.
func TestParse_ChecklistVacioNoDivideEntreCero(t *testing.T) {
	raw := `# ODD: gestion-inventario

> **Estado:** activo

## Checklist accionable

_(pendiente de completar)_

## Criterios de aceptación

- [ ] Este criterio tiene una casilla, pero el checklist sigue vacío
`

	doc, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse() devolvió error inesperado: %v", err)
	}

	if doc.Progress.Total != 0 {
		t.Errorf("Progress.Total = %d, se esperaba 0", doc.Progress.Total)
	}
	if doc.Progress.Percent != 0 {
		t.Errorf("Progress.Percent = %v, se esperaba 0", doc.Progress.Percent)
	}
}

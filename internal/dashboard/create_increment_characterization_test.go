package dashboard

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// plantillaEsperadaConIntento es la plantilla vigente que CreateIncrement produce
// cuando se aporta un Intent explícito, transcrita byte a byte de la salida real
// del binario sin modificar (D-03). El campo Type NO participa en el cuerpo
// generado: este mismo literal es también el esperado para "tipo vacío no altera
// el cuerpo".
const plantillaEsperadaConIntento = `# Propuesta: Mi Cambio (mi-cambio)

## Propósito (Intent)

Propósito X

---

## Alcance (Scope)

### Dentro de Alcance (In Scope)
- Diseño, implementación y verificación de las capacidades de mi-cambio.
- Cobertura de pruebas unitarias y validación formal de requerimientos.

### Fuera de Alcance (Out of Scope)
- Cambios no relacionados directamente con los objetivos de esta iteración.

---

## Capacidades (Capabilities)

### Nuevas Capacidades
- ` + "`mi-cambio`" + `: Funcionalidad principal introducida por el cambio.

---

## Enfoque de Implementación (Approach)
1. Exploración y definición de especificaciones con escenarios BDD en spec.md.
2. Diseño técnico detallado y arquitectura en design.md.
3. Desglose y seguimiento de tareas en tasks.md.
4. Verificación formal y consolidación de documentación viva en archive.
`

// plantillaEsperadaSinIntento es la plantilla vigente cuando Intent llega vacío:
// el único cambio frente a plantillaEsperadaConIntento es el texto por defecto
// del Propósito, transcrito byte a byte de la misma forma.
const plantillaEsperadaSinIntento = `# Propuesta: Mi Cambio (mi-cambio)

## Propósito (Intent)

Implementación e integración de la funcionalidad mi-cambio bajo la metodología SDD.

---

## Alcance (Scope)

### Dentro de Alcance (In Scope)
- Diseño, implementación y verificación de las capacidades de mi-cambio.
- Cobertura de pruebas unitarias y validación formal de requerimientos.

### Fuera de Alcance (Out of Scope)
- Cambios no relacionados directamente con los objetivos de esta iteración.

---

## Capacidades (Capabilities)

### Nuevas Capacidades
- ` + "`mi-cambio`" + `: Funcionalidad principal introducida por el cambio.

---

## Enfoque de Implementación (Approach)
1. Exploración y definición de especificaciones con escenarios BDD en spec.md.
2. Diseño técnico detallado y arquitectura en design.md.
3. Desglose y seguimiento de tareas en tasks.md.
4. Verificación formal y consolidación de documentación viva en archive.
`

// TestCreateIncrement_PlantillaVigenteSinCuerpoSembrado es la puerta de control
// de severidad alta del riesgo R3 (REQ-15.1): caracteriza, byte a byte, el
// comportamiento vigente de CreateIncrement cuando no se aporta ProposalBody.
// Debe permanecer en VERDE, sin tocar sus literales esperados, durante y
// después de la extensión de la tarea 4.5. Comparación siempre por igualdad de
// cadena completa (`!=`), nunca por subcadena: una reordenación o pérdida de
// línea en blanco debe hacerla fallar.
func TestCreateIncrement_PlantillaVigenteSinCuerpoSembrado(t *testing.T) {
	tests := []struct {
		name string
		req  CreateIncrementRequest
		want string
	}{
		{
			name: "intento y tipo explícitos",
			req:  CreateIncrementRequest{Name: "mi-cambio", Intent: "Propósito X", Type: "feature"},
			want: plantillaEsperadaConIntento,
		},
		{
			name: "intento vacío usa el texto por defecto",
			req:  CreateIncrementRequest{Name: "mi-cambio"},
			want: plantillaEsperadaSinIntento,
		},
		{
			name: "tipo vacío no altera el cuerpo",
			req:  CreateIncrementRequest{Name: "mi-cambio", Intent: "Propósito X"},
			want: plantillaEsperadaConIntento,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(t.TempDir())
			res, err := svc.CreateIncrement(tt.req)
			if err != nil {
				t.Fatalf("CreateIncrement() error = %v", err)
			}
			got, err := os.ReadFile(res.Path)
			if err != nil {
				t.Fatalf("leyendo proposal.md: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("CreateIncrement() produjo bytes distintos de la plantilla vigente.\n--- obtenido ---\n%s\n--- esperado ---\n%s", string(got), tt.want)
			}
		})
	}
}

// cuerpoSembradoDePrueba simula el cuerpo ya renderizado que la promoción ODD→SDD
// (Fase 5, fuera de este lote) enviaría en ProposalBody. Deliberadamente distinto
// de la forma de la plantilla vigente, para que una confusión entre ambos cuerpos
// sea detectable por igualdad de cadena.
const cuerpoSembradoDePrueba = `# Propuesta: Gestión de Inventario (gestion-inventario)

> **Promovido desde el documento vivo ODD** ` + "`odd/tasks/gestion-inventario.md`" + ` el 2026-09-17.

## Propósito (Intent)

Cuerpo íntegro ya renderizado por la promoción ODD, sin plantilla genérica.
`

// TestCreateIncrement_ProposalBodySembrado (tareas 4.2/4.4) verifica que, cuando
// la petición aporta ProposalBody con contenido no vacío, CreateIncrement escribe
// esos bytes de forma literal en proposal.md, sin sustituirlos por la plantilla
// generada [D-02].
func TestCreateIncrement_ProposalBodySembrado(t *testing.T) {
	svc := NewService(t.TempDir())
	req := CreateIncrementRequest{
		Name:         "gestion-inventario",
		Intent:       "Propósito irrelevante para este caso: ProposalBody manda",
		ProposalBody: cuerpoSembradoDePrueba,
	}

	res, err := svc.CreateIncrement(req)
	if err != nil {
		t.Fatalf("CreateIncrement() error = %v", err)
	}

	got, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatalf("leyendo proposal.md: %v", err)
	}
	if string(got) != cuerpoSembradoDePrueba {
		t.Errorf("CreateIncrement() no escribió ProposalBody de forma literal.\n--- obtenido ---\n%s\n--- esperado ---\n%s", string(got), cuerpoSembradoDePrueba)
	}
}

// TestCreateIncrement_HTTPProposalBodyRegression (tareas 4.6/4.7) es la
// regresión de extremo a extremo de REQ-15.1 sobre el endpoint ya existente
// POST /api/increments: sin proposal_body debe seguir produciendo la plantilla
// vigente; con proposal_body debe producir exactamente esos bytes. Reutiliza
// los mismos literales que las pruebas unitarias anteriores (sin duplicarlos).
func TestCreateIncrement_HTTPProposalBodyRegression(t *testing.T) {
	svc := NewService(t.TempDir())
	server := NewServer(svc)
	router := server.Router()

	tests := []struct {
		name string
		body CreateIncrementRequest
		want string
	}{
		{
			name: "sin proposal_body produce la plantilla vigente",
			body: CreateIncrementRequest{Name: "mi-cambio", Intent: "Propósito X", Type: "feature"},
			want: plantillaEsperadaConIntento,
		},
		{
			name: "con proposal_body produce exactamente esos bytes",
			body: CreateIncrementRequest{
				Name:         "gestion-inventario",
				Intent:       "Propósito irrelevante para este caso: ProposalBody manda",
				ProposalBody: cuerpoSembradoDePrueba,
			},
			want: cuerpoSembradoDePrueba,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody, err := json.Marshal(tt.body)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/increments", bytes.NewReader(reqBody))
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if rr.Code != http.StatusCreated {
				t.Fatalf("POST /api/increments esperado 201 Created, obtenido %d: %s", rr.Code, rr.Body.String())
			}

			var res CreateIncrementResponse
			if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
				t.Fatalf("error decodificando respuesta de /api/increments: %v", err)
			}

			got, err := os.ReadFile(res.Path)
			if err != nil {
				t.Fatalf("leyendo proposal.md: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("POST /api/increments produjo bytes distintos de lo esperado.\n--- obtenido ---\n%s\n--- esperado ---\n%s", string(got), tt.want)
			}
		})
	}
}

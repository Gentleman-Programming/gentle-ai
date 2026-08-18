package repository

import "testing"

func TestSlugify(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		// Registry rows from docs/repository-registry.md (Slice 1).
		{"gp-apps-cross Pagos", "gp-apps-cross/Pagos", "gp-apps-cross-pagos"},
		{"SmartClic MSPagos", "SmartClic/MSPagos", "smartclic-mspagos"},
		{"SmartClic cliente-hub-front", "SmartClic/cliente-hub-front", "smartclic-cliente-hub-front"},
		{"gp-apps-cross portal-sr-front", "gp-apps-cross/portal-sr-front", "gp-apps-cross-portal-sr-front"},
		{"SmartClic erp-mf-root-config", "SmartClic/erp-mf-root-config", "smartclic-erp-mf-root-config"},
		{"SmartClic erp-mf-comun", "SmartClic/erp-mf-comun", "smartclic-erp-mf-comun"},
		{"SmartClic erp-mf-configuracion", "SmartClic/erp-mf-configuracion", "smartclic-erp-mf-configuracion"},
		{"SmartClic erp-mf-configuraciones", "SmartClic/erp-mf-configuraciones", "smartclic-erp-mf-configuraciones"},
		{"SmartClic erp-mf-estilos", "SmartClic/erp-mf-estilos", "smartclic-erp-mf-estilos"},
		{"SmartClic erp-mf-header", "SmartClic/erp-mf-header", "smartclic-erp-mf-header"},
		{"SmartClic erp-mf-home", "SmartClic/erp-mf-home", "smartclic-erp-mf-home"},
		{"SmartClic erp-mf-logistica", "SmartClic/erp-mf-logistica", "smartclic-erp-mf-logistica"},
		{"SmartClic erp-mf-menu", "SmartClic/erp-mf-menu", "smartclic-erp-mf-menu"},
		{"SmartClic erp-mf-punto-venta", "SmartClic/erp-mf-punto-venta", "smartclic-erp-mf-punto-venta"},
		{"SmartClic erp-mf-punto-venta-menu", "SmartClic/erp-mf-punto-venta-menu", "smartclic-erp-mf-punto-venta-menu"},
		{"SmartClic erp-mf-resources", "SmartClic/erp-mf-resources", "smartclic-erp-mf-resources"},
		{"SmartClic erp-mf-seguridad", "SmartClic/erp-mf-seguridad", "smartclic-erp-mf-seguridad"},
		{"SmartClic erp-mf-tiendalink", "SmartClic/erp-mf-tiendalink", "smartclic-erp-mf-tiendalink"},
		{"SReasonsERP ERPLogistica", "SReasonsERP/ERPLogistica", "sreasonserp-erplogistica"},
		{"SReasonsERP ERPPlanillas", "SReasonsERP/ERPPlanillas", "sreasonserp-erpplanillas"},
		{"SmartClic ERPBalanceContable", "SmartClic/ERPBalanceContable", "smartclic-erpbalancecontable"},
		{"GP-GCG erptalleres", "GP-GCG/erptalleres", "gp-gcg-erptalleres"},
		{"SmartClic ERPIntegracion", "SmartClic/ERPIntegracion", "smartclic-erpintegracion"},
		{"SmartClic ERPFinanzasCore", "SmartClic/ERPFinanzasCore", "smartclic-erpfinanzascore"},
		{"SmartClic erpintegracionsunat", "SmartClic/erpintegracionsunat", "smartclic-erpintegracionsunat"},
		{"Gentleman-Programming gentle-ai", "Gentleman-Programming/gentle-ai", "gentleman-programming-gentle-ai"},

		// Additional cases per SPEC-005 / design Decision 2.
		{"leading/trailing whitespace trimmed", "  gp-apps-cross/Pagos  ", "gp-apps-cross-pagos"},
		{"runs of separators collapse to one dash", "gp-apps-cross///Pagos", "gp-apps-cross-pagos"},
		{"mixed non-alnum separators collapse", "gp_apps.cross Pagos!!", "gp-apps-cross-pagos"},
		{"leading and trailing separators trimmed", "/gp-apps-cross/Pagos/", "gp-apps-cross-pagos"},
		{"underscores collapse like other separators", "SmartClic_MSPagos", "smartclic-mspagos"},
		{"empty input rejected", "", ""},
		{"whitespace-only input rejected", "   ", ""},
		{"separator-only input rejected", "///---___", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Slugify(tt.in)
			if got != tt.want {
				t.Errorf("Slugify(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSlugifyIdempotent(t *testing.T) {
	inputs := []string{
		"gp-apps-cross/Pagos",
		"SmartClic/erp-mf-punto-venta-menu",
		"Gentleman-Programming/gentle-ai",
		"",
		"gp_apps.cross Pagos!!",
	}

	for _, in := range inputs {
		first := Slugify(in)
		second := Slugify(first)
		if first != second {
			t.Errorf("Slugify not idempotent for %q: first=%q second=%q", in, first, second)
		}
	}
}

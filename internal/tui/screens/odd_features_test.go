package screens

import "testing"

// TestODDFeaturesActionAt cubre el resolvedor tipado cursor → acción de la
// pantalla ODD para 0, 1 y N features, incluida la fila de cada feature y
// las cinco acciones globales que siempre van detrás en el mismo orden fijo
// [D-12]: nada en el paquete debe repetir esta aritmética de índices.
func TestODDFeaturesActionAt(t *testing.T) {
	oneFeature := []ODDFeatureInfo{
		{Feature: "gestion-inventario", Status: "activo"},
	}
	twoFeatures := []ODDFeatureInfo{
		{Feature: "gestion-inventario", Status: "activo"},
		{Feature: "modulo-pagos", Status: "promovido", PromotedTo: "modulo-pagos"},
	}

	tests := []struct {
		name       string
		features   []ODDFeatureInfo
		cursor     int
		wantAction ODDAction
		wantIndex  int
	}{
		{name: "0 features: cursor negativo no resuelve ninguna acción", features: nil, cursor: -1, wantAction: ODDActionNone, wantIndex: -1},
		{name: "0 features: primera posición es Crear", features: nil, cursor: 0, wantAction: ODDActionCreate, wantIndex: -1},
		{name: "0 features: segunda posición es Promover", features: nil, cursor: 1, wantAction: ODDActionPromote, wantIndex: -1},
		{name: "0 features: tercera posición es Comprobar espejo", features: nil, cursor: 2, wantAction: ODDActionCheckMirror, wantIndex: -1},
		{name: "0 features: cuarta posición es Ir al carril SDD", features: nil, cursor: 3, wantAction: ODDActionGoToSDDLane, wantIndex: -1},
		{name: "0 features: quinta posición es Volver", features: nil, cursor: 4, wantAction: ODDActionBack, wantIndex: -1},
		{name: "0 features: posición fuera de rango no resuelve ninguna acción", features: nil, cursor: 5, wantAction: ODDActionNone, wantIndex: -1},

		{name: "1 feature: cursor 0 selecciona la única feature", features: oneFeature, cursor: 0, wantAction: ODDActionSelectFeature, wantIndex: 0},
		{name: "1 feature: cursor 1 es Crear", features: oneFeature, cursor: 1, wantAction: ODDActionCreate, wantIndex: -1},
		{name: "1 feature: cursor 2 es Promover", features: oneFeature, cursor: 2, wantAction: ODDActionPromote, wantIndex: -1},
		{name: "1 feature: cursor 3 es Comprobar espejo", features: oneFeature, cursor: 3, wantAction: ODDActionCheckMirror, wantIndex: -1},
		{name: "1 feature: cursor 4 es Ir al carril SDD", features: oneFeature, cursor: 4, wantAction: ODDActionGoToSDDLane, wantIndex: -1},
		{name: "1 feature: cursor 5 es Volver", features: oneFeature, cursor: 5, wantAction: ODDActionBack, wantIndex: -1},
		{name: "1 feature: cursor fuera de rango no resuelve ninguna acción", features: oneFeature, cursor: 6, wantAction: ODDActionNone, wantIndex: -1},

		{name: "N features: cursor 0 selecciona la primera feature", features: twoFeatures, cursor: 0, wantAction: ODDActionSelectFeature, wantIndex: 0},
		{name: "N features: cursor 1 selecciona la segunda feature (promovida)", features: twoFeatures, cursor: 1, wantAction: ODDActionSelectFeature, wantIndex: 1},
		{name: "N features: cursor 2 es Crear", features: twoFeatures, cursor: 2, wantAction: ODDActionCreate, wantIndex: -1},
		{name: "N features: cursor 3 es Promover", features: twoFeatures, cursor: 3, wantAction: ODDActionPromote, wantIndex: -1},
		{name: "N features: cursor 4 es Comprobar espejo", features: twoFeatures, cursor: 4, wantAction: ODDActionCheckMirror, wantIndex: -1},
		{name: "N features: cursor 5 es Ir al carril SDD", features: twoFeatures, cursor: 5, wantAction: ODDActionGoToSDDLane, wantIndex: -1},
		{name: "N features: cursor 6 es Volver", features: twoFeatures, cursor: 6, wantAction: ODDActionBack, wantIndex: -1},
		{name: "N features: cursor fuera de rango no resuelve ninguna acción", features: twoFeatures, cursor: 7, wantAction: ODDActionNone, wantIndex: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAction, gotIndex := ODDFeaturesActionAt(tt.features, tt.cursor)
			if gotAction != tt.wantAction {
				t.Errorf("ODDFeaturesActionAt() acción = %v, se esperaba %v", gotAction, tt.wantAction)
			}
			if gotIndex != tt.wantIndex {
				t.Errorf("ODDFeaturesActionAt() índice = %d, se esperaba %d", gotIndex, tt.wantIndex)
			}
		})
	}
}

// TestODDFeaturesOptionsCursorBoundsMatchActionAt cubre, dentro de este
// paquete, la invariante que la tarea 7.1 exige frente a screenOptionCount:
// toda posición de cursor entre 0 y len(ODDFeaturesOptions(features))-1 debe
// resolver una acción distinta de ODDActionNone, y la posición inmediatamente
// posterior a la última opción debe caer en ODDActionNone. La comprobación
// equivalente sobre el recuento real de opciones de internal/tui/model.go
// llega con el cableado de la pantalla (tarea 7.8), fuera de este lote.
func TestODDFeaturesOptionsCursorBoundsMatchActionAt(t *testing.T) {
	tests := []struct {
		name        string
		features    []ODDFeatureInfo
		wantOptions int
	}{
		{name: "0 features", features: nil, wantOptions: 5},
		{name: "1 feature", features: []ODDFeatureInfo{{Feature: "gestion-inventario", Status: "activo"}}, wantOptions: 6},
		{
			name: "N features",
			features: []ODDFeatureInfo{
				{Feature: "gestion-inventario", Status: "activo"},
				{Feature: "modulo-pagos", Status: "promovido", PromotedTo: "modulo-pagos"},
				{Feature: "reporteria-financiera", Status: "activo"},
			},
			wantOptions: 8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := ODDFeaturesOptions(tt.features)
			if len(options) != tt.wantOptions {
				t.Fatalf("ODDFeaturesOptions() devolvió %d opciones, se esperaban %d", len(options), tt.wantOptions)
			}

			for cursor := 0; cursor < len(options); cursor++ {
				if action, _ := ODDFeaturesActionAt(tt.features, cursor); action == ODDActionNone {
					t.Errorf("cursor %d cae dentro de las %d opciones pero resolvió ODDActionNone", cursor, len(options))
				}
			}

			if action, _ := ODDFeaturesActionAt(tt.features, len(options)); action != ODDActionNone {
				t.Errorf("cursor %d (fuera de las %d opciones) resolvió %v, se esperaba ODDActionNone", len(options), len(options), action)
			}
		})
	}
}

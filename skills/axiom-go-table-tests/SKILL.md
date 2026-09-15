# Convención de Pruebas Tabulares en Go (Table-Driven Tests)

description: "Estandarización de pruebas unitarias idiomáticas en Go mediante estructuras anónimas y subpruebas t.Run"
trigger: "Al crear o modificar suites de pruebas unitarias *_test.go en paquetes Go"
origin: "mined"

---

## Propósito

Asegurar que todas las pruebas unitarias en Go mantengan alta legibilidad, cobertura de casos borde y diagnóstico rápido de fallos mediante el patrón idiomático de Table-Driven Tests recomendado por el equipo de Go y adoptado canónicamente en Axiom.

## Reglas Obligatorias

1. **Estructura de Casos:** Definir los casos de prueba dentro de un slice de structs anónimos con nombres descriptivos:
   ```go
   tests := []struct {
       name     string
       input    string
       expected string
       wantErr  bool
   }{
       {
           name:     "caso exitoso nominal",
           input:    "valido",
           expected: "resultado",
           wantErr:  false,
       },
   }
   ```

2. **Aislamiento con `t.Run`:** Ejecutar cada caso en su propia subprueba nombrada:
   ```go
   for _, tt := range tests {
       t.Run(tt.name, func(t *testing.T) {
           got, err := FuncionBajoPrueba(tt.input)
           if (err != nil) != tt.wantErr {
               t.Fatalf("FuncionBajoPrueba() error = %v, wantErr %v", err, tt.wantErr)
           }
           if got != tt.expected {
               t.Errorf("FuncionBajoPrueba() = %v, esperado %v", got, tt.expected)
           }
       })
   }
   ```

3. **Cero Salidas Globales:** No usar `os.Exit` ni `panic` dentro de las funciones de prueba; emplear `t.Errorf` para fallos no fatales y `t.Fatalf` para precondiciones.

# Manejo y Envoltorio Idiomático de Errores en Go (%w)

description: "Uso estricto de fmt.Errorf con el especificador %w para preservación de cadenas de error y errores centinela"
trigger: "Al capturar, propagar o formatear errores en cualquier paquete Go"
origin: "mined"

---

## Propósito

Preservar el contexto de ejecución y la causa raíz de los errores a través de las capas del sistema, permitiendo la inspección mediante `errors.Is` y `errors.As`.

## Reglas Obligatorias

1. **Envoltorio Contextual:** Siempre envolver los errores salientes con contexto:
   ```go
   if err != nil {
       return nil, fmt.Errorf("error cargando configuración desde %s: %w", path, err)
   }
   ```

2. **Prohibido %v o %s para Errores Propagados:** No usar `%v` o `%s` cuando se requiere inspección de causas raíz; reservar `%w` para envolver la interfaz `error`.

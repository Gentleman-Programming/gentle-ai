# Tareas: Nueva Persona Axiom, Contrato Lingüístico en Español Peninsular y Desactivación del Voseo (INC-10)

## Fase 1: Identificador de Modelo y Configuración CLI/TUI (`internal/model/`, `internal/cli/`, `internal/tui/`)

- [x] T-01 Añadir `PersonaAxiom PersonaID = "axiom"` en `internal/model/types.go`.
- [x] T-02 Actualizar `internal/tui/screens/persona.go` y `internal/tui/model.go` para que `PersonaAxiom` figure en `PersonaOptions()` y sea seleccionada por defecto con su descripción en castellano peninsular.
- [x] T-03 Actualizar `normalizePersona` en `internal/cli/validate.go` para admitir `model.PersonaAxiom` y resolver `axiom` como valor por defecto cuando no se especifique `--persona`.

## Fase 2: Creación de Assets de Prompt de la Persona Axiom (`internal/assets/`)

- [x] T-04 Crear `internal/assets/generic/persona-axiom.md` con la voz de Arquitecto Senior en castellano peninsular de España, prohibición estricta de voseo y directiva de artefactos técnicos en español.
- [x] T-05 Crear `internal/assets/claude/persona-axiom.md` y `internal/assets/claude/output-style-axiom.md` para Claude Code.
- [x] T-06 Crear `internal/assets/opencode/persona-axiom.md` para OpenCode/Kilocode y `internal/assets/kiro/persona-axiom.md` para Kiro IDE.
- [x] T-07 Crear `internal/assets/hermes/persona-axiom.md` para Hermes Agent y `internal/assets/kimi/output-style-axiom.md` para Kimi IDE.

## Fase 3: Cableado del Componente de Persona (`internal/components/persona/`)

- [x] T-08 Actualizar `internal/components/persona/resources.go` para registrar el estilo de salida de Axiom (`axiom.md`) y configurar `ResourcePlanFor` para escribirlo y retirar estilos retirados.
- [x] T-09 Actualizar `internal/components/persona/inject.go` para mapear `PersonaAxiom` al contenido correspondiente y soportar su inyección en Claude, OpenCode, Kiro, Hermes y Kimi.

## Fase 4: Reforma del Contrato Lingüístico SDD (`internal/assets/skills/`)

- [x] T-10 Actualizar la sección `Language Domain Contract` en `internal/assets/skills/_shared/sdd-orchestrator-sections.md` para prescribir artefactos SDD en español castellano en proyectos Axiom.
- [x] T-11 Actualizar las referencias de `Language Domain Contract` en las skills de SDD (`sdd-apply`, `sdd-verify`, etc.) y prompts de orquestación donde aplique.

## Fase 5: Pruebas, Golden Files y Verificación Formal

- [x] T-12 Implementar pruebas unitarias de contrato lingüístico y selección de `PersonaAxiom` en `internal/components/persona/persona_language_contract_test.go`, `internal/tui/screens/persona_language_contract_test.go` y assets.
- [x] T-13 Actualizar los golden files y tests de drift/ratchet afectados por el cambio en `Language Domain Contract`.
- [x] T-14 Ejecutar `go test ./...` para verificar toda la suite en verde.
- [x] T-15 Validar con `gentle-ai sdd-verify-validate`, compilar el binario y generar `verify-report.md` y `archive-report.md` para archivar INC-10.

# SDD Explore: Human Gates Programáticos

## Descubrimiento
El proceso SDD de *gentle-ai* tiene definidos conceptualmente tres *Human Gates*:
1. Scope (después de Propose, antes de Spec/Design)
2. Technical (después de Spec/Design, antes de Tasks)
3. Pre-Implementation (después de Tasks, antes de Apply)

Actualmente, estos gates solo existen como instrucciones en prosa para los agentes. El sistema `status.go` no los reconoce; avanza automáticamente de fase (`resolveNextRecommended`) si el artefacto de la fase anterior existe (ej. si hay `proposal.md`, recomienda `spec`). Esto permite que un modo Automático se salte las revisiones humanas críticas.

## Implicaciones
Si implementamos la lectura de marcadores de aprobación, el motor central de dependencias bloqueará el progreso. Esto forzará al orquestador a detenerse y pedir el gate correspondiente, fortaleciendo el ciclo "Humano-en-el-bucle" (HITL) que exige la arquitectura.

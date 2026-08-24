<!--
PLANTILLA: skills/methodology/<process-id>/SKILL.md
Categoría cognitiva: "El Cómo" — el PROCESO paso a paso que el agente debe
seguir, independiente de qué tecnología o repo esté tocando. No dice QUÉ
tecnología usar (eso es technology/) ni QUÉ significa para el negocio (eso
es domain/) — dice cómo debe razonar y en qué orden debe actuar.

Reemplaza cada bloque [ENTRE CORCHETES]. Cada paso debe ser verificable: un
dev-verifier (o un humano) debe poder confirmar que el paso realmente
ocurrió, no solo leer que "debería" haber ocurrido.
-->
---
name: [process-id]                     # ej. sdd-exploration, sdd-apply, strict-tdd-mode
description: "Trigger: [process-id]. Methodology rules for [una frase: qué garantiza este proceso]."
license: Apache-2.0
metadata:
  author: [tu nombre]
  version: "1.0"
---

# Methodology: [Nombre legible del proceso]

## 1. Cuándo se activa
[Qué fase/tarea/trigger hace que un agente deba seguir esta metodología.
Ejemplo: "Se activa cuando dev-orchestrator clasifica un cambio como
EXISTING y delega a un Explorer."]

## 2. Objetivo
[Qué garantiza este proceso que NO pasaría si el agente improvisara.
Ejemplo: "Evita que el agente empiece a modificar código sin haber mapeado
antes las pruebas y componentes reutilizables existentes."]

## 3. Entradas requeridas
- [Qué artefacto/contexto debe existir ANTES de empezar. Si algo falta, el
  agente debe detenerse y reportarlo, no improvisar.]

## 4. Pasos obligatorios
1. [Paso verificable, en orden estricto]
2. [Paso verificable]
3. [Paso verificable]

> Si el agente se salta un paso, DEBE declararlo explícitamente en su
> resultado (`skill_resolution` o equivalente), nunca omitirlo en silencio.

## 5. Criterios de salida (Definition of Done)
- [ ] [Condición verificable 1]
- [ ] [Condición verificable 2]

## 6. Errores comunes a evitar
- ❌ [Error real que ya ocurrió o es previsible] → ✅ [cómo evitarlo]

## 7. Artefactos que produce
[Qué escribe este proceso al terminar (ej. topic_key de Engram, archivo,
sección del resultado estructurado) y quién lo consume después.]

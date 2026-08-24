<!--
PLANTILLA: skills/policy/<policy-id>/SKILL.md
Categoría cognitiva: "Los Límites" — invariantes GLOBALES que nunca deben
romperse, sin importar repositorio, metodología o tecnología. Jerarquía de
autoridad de este sistema: policy > repo-profile > domain > methodology >
technology > preferencia del agente. Una skill de technology/ JAMÁS puede
justificar violar una policy.

Reemplaza cada bloque [ENTRE CORCHETES]. Cada regla debe ser una prohibición
u obligación sin ambigüedad ("NUNCA X" / "SIEMPRE Y"), nunca una sugerencia.
-->
---
name: [policy-id]                    # ej. security, secrets-management, git-isolation-strategy
description: "Trigger: [policy-id]. Policy rules for [una frase: qué protege esta política]."
license: Apache-2.0
metadata:
  author: [tu nombre]
  version: "1.0"
---

# Policy: [Nombre legible de la política]

## 1. Alcance
[A qué agentes/repos/tecnologías aplica esta política. Por defecto es
GLOBAL — si tiene excepciones de alcance, decláralas aquí explícitamente.]

## 2. Reglas obligatorias
1. NUNCA [acción prohibida, específica y verificable — ej. "NUNCA
   almacenar tokens, contraseñas o secretos en código fuente o fixtures de
   prueba"].
2. SIEMPRE [acción obligatoria, específica].
3. [Regla 3]

## 3. Ejemplos de violación
- ❌ [Situación/código real que viola la regla 1, con el porqué exacto]
- ❌ [Situación que viola otra regla]

## 4. Excepciones permitidas
[Si existen casos legítimos de excepción, lista exactamente cuáles y quién
los autoriza (ej. "solo con label size:exception aprobado por un
mantenedor"). Si NO hay excepciones posibles, escribe explícitamente
"Sin excepciones — esta regla es absoluta."]

## 5. Consecuencia de violar la regla
[Qué debe pasar cuando un agente detecta que va a violar esta policy:
¿detenerse y escalar? ¿rechazar con error tipado? ¿pedir Human Gate?
Sé explícito, igual que un `strict enforcement` en código.]

## 6. Precedencia frente a otras skills
[Recordatorio explícito de qué le gana a qué si hay conflicto — por
ejemplo: "Si `technology/[x]` recomienda algo que esta policy prohíbe,
esta policy gana siempre, sin excepción."]

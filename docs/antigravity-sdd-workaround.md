# Antigravity IDE: SDD Single-Agent Workaround

## El Problema
Antigravity IDE actualmente no soporta la invocación nativa de subagentes en background (multi-threading de LLMs). Todas las fases de Spec-Driven Development (SDD) deben ejecutarse secuencialmente en el mismo hilo de conversación.
Esto genera un alto riesgo de **degradación de contexto y alucinaciones**, ya que el LLM empieza a mezclar instrucciones de skills anteriores y pierde el hilo de la arquitectura.

## La Solución: Artifact-Driven State Machine
Para implementar una integración temprana sin depender de APIs externas y **sin afectar la arquitectura multi-agente original de proyectos como Cursor u OpenCode**, aplicamos un patrón de Máquina de Estados apoyada estrictamente en el File System local.

### Reglas a Inyectar (Específicas para Antigravity)

1. **Role Switching Estricto**
   El orquestador debe anunciar el cambio de fase y cargar el skill correspondiente (`SKILL.md`) en su contexto, ignorando temporalmente directivas previas.

2. **File-System como Memoria (Save State)**
   Al terminar una fase (ej. `sdd-propose`), Antigravity tiene PROHIBIDO avanzar sin antes guardar el output completo en un archivo físico (ej. `.sdd/propuesta.md`). El chat NO es un medio de almacenamiento confiable.

3. **Amnesia Controlada (Load State)**
   Al iniciar la siguiente fase (ej. `sdd-spec`), Antigravity NO DEBE confiar en su historial de chat. Su primera acción obligatoria es usar la herramienta de lectura (`Read`) para cargar el archivo generado en el paso anterior. Esto refresca el contexto exacto necesario para la fase actual.

4. **Uso Correcto de Engram**
   Engram (`mem_save`) se preserva ÚNICAMENTE para registrar decisiones arquitectónicas globales, convenciones y bugfixes. NO debe usarse para guardar el estado intermedio de un SDD en curso (para eso están los archivos `.sdd/*.md`).

## Conclusión
Este workaround permite tener SDD funcional en Antigravity hoy mismo, operando bajo un modo de "Single-Threaded Simulation", manteniendo la limpieza y modularidad del repositorio original de Gentleman intactas.

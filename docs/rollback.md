# Copias de seguridad y restauración

Axiom crea una copia de los ficheros de configuración que va a modificar antes de instalar, sincronizar o actualizar componentes. La pantalla **Respaldos** de la TUI distingue las copias de Axiom de las copias históricas de Gentle AI: selecciona por defecto la copia más reciente de Axiom y mantiene las de Gentle AI disponibles para una restauración deliberada.

## Qué se guarda

Cada operación de escritura:

1. Calcula la huella de los ficheros incluidos en la copia.
2. Omite el respaldo si su contenido coincide con el último.
3. Crea una instantánea comprimida (`snapshot.tar.gz`).
4. Aplica la política de retención a las copias no fijadas.

El manifiesto registra el origen, la fecha, el número de ficheros, la huella y si una ruta existía antes de la operación. Las copias antiguas, anteriores al formato comprimido, conservan un directorio `files/`; Axiom también las admite al restaurar.

El alcance de un respaldo de sincronización depende de los agentes registrados como instalados por Axiom. No presupongas que incluye los directorios de agentes configurados manualmente fuera de Axiom. Las copias de Axiom (`~/.axiom/backups/`) y las históricas de Gentle AI (`~/.gentle-ai/backups/`) mantienen su procedencia, aunque tengan identificadores repetidos.

## Retención

| Ajuste | Valor predeterminado | Comportamiento |
|--------|---------------------|----------------|
| Cantidad | 5 | Se conservan las cinco copias no fijadas más recientes en cada carpeta de respaldos |
| Copias fijadas | Sin límite | No se eliminan durante la poda automática |
| Duplicados | Omitidos | No se crea otra copia si la configuración no ha cambiado |
| Compresión | Activada | Las nuevas copias usan `tar.gz` |

## Restaurar desde la TUI

Abre la pantalla de respaldos:

```sh
axiom tui
```

En la pantalla **Respaldos**, utiliza `j`/`k` para elegir una copia y `Enter` para restaurarla. La selección inicial apunta a la copia de Axiom más reciente; elige expresamente una copia identificada como Gentle AI solo si quieres recuperar ese estado histórico.

| Tecla | Acción |
|-------|--------|
| `j` / `k` | Mover la selección |
| `Enter` | Restaurar la copia seleccionada |
| `p` | Fijar o quitar la fijación |
| `r` | Añadir o cambiar la descripción |
| `d` | Eliminar la copia seleccionada |
| `Esc` | Volver |

La interfaz conserva la raíz/origen seleccionado al restaurar; no deduzcas el origen solo por el ID, porque puede repetirse entre Axiom y Gentle AI.

## Semántica de restauración

- Si `existed=true`, restaura el fichero en su ruta original.
- Si `existed=false`, elimina el fichero creado por la operación respaldada.
- Las escrituras de cada fichero son atómicas; la restauración completa puede abarcar varios ficheros.
- Se admiten tanto instantáneas `tar.gz` como el formato histórico `files/`.
- La restauración revierte ficheros de configuración; no desinstala paquetes del sistema.

## Si falla una verificación

1. Lee qué comprobaciones han fallado y qué ficheros se han modificado.
2. Abre la TUI y restaura la copia de Axiom apropiada desde **Respaldos**.
3. Corrige la causa externa y valida el plan antes de repetir la operación con `axiom install --dry-run` o `axiom sync --dry-run`.
4. Aplica de nuevo la operación solo cuando el plan sea el esperado.

Para escoger un respaldo de un origen concreto, usa la pantalla **Respaldos**: el atajo de CLI `axiom restore latest` tiene una resolución independiente y no ofrece la misma preselección de origen de la TUI.

## Límites

- Las instalaciones realizadas mediante `brew`, `apt-get`, `pacman` o `dnf` no se desinstalan al restaurar una copia de configuración. Usa el gestor de paquetes correspondiente si también quieres retirarlas.
- No selecciones una copia histórica de Gentle AI por ser la más reciente en conjunto: comprueba el origen que muestra la TUI antes de confirmar.

# Committed review boundaries

When STATUS derives a committed range from a clean worktree, it starts at the
branch's integration point. A successful approval acknowledgement can move that
boundary forward. For example, if `O..A` and then `A..B` were acknowledged, the
next non-passive change at `C` is offered as `B..C`, not `A..C`. STATUS names the
base it selected in the offered START command.

An explicit `--base-ref O --committed-only` selects `O..C`. STATUS does not
replace that selector with the automatic boundary. Callers asking for a
work-unit review should pass the last acknowledged boundary, not reuse the
original branch point. A passive tail can still suppress a fresh offer under
the existing passive-content rule; it does not change the selected range.

Only exact acknowledgement evidence can advance the reviewed boundary. An
unresolved or rejected verdict does not count, even after another commit. The
boundary only avoids repeat review work; it grants neither approval nor delivery
authority for a subsequent candidate.

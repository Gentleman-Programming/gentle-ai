# Applicability-Driven Threat Matrix

Use this matrix only when the design changes routing, shell commands, subprocesses, version-control automation, PR automation, executable-file classification, or process integration. Mark each row `Applicable` or explicit `N/A` with a reason. Do not invent tasks or tests for `N/A` rows.

| Boundary | Minimum adversarial cases | Applicability | Design response | Planned RED tests |
|---|---|---|---|---|
| Documentation-like paths | `requirements.txt`, `CMakeLists.txt`, executable Markdown/MDX, `README.sh` | Applicable / N/A: reason | Classification and execution boundary | One test per applicable class |
| Git repository selection | `git -C`, relative paths, absolute paths | Applicable / N/A: reason | Repository/cwd authority | One test per applicable selector |
| Commit state | staged, `commit -a`, empty index | Applicable / N/A: reason | Index/worktree semantics | One test per applicable state |
| Push state | tracking branch, first push, explicit refspec | Applicable / N/A: reason | Destination/ref resolution | One test per applicable state |
| PR commands | explicit `--head`, environment prefix, composed commands | Applicable / N/A: reason | Argument composition and ownership | One test per applicable form |

For every applicable row, define the expected safe behavior, failure behavior, and concrete test boundary. Carry those cases unchanged into `tasks.md`; implementation writes the mapped RED tests before production changes. If the change has no routing/shell/process boundary, record the matrix as not applicable rather than expanding it.

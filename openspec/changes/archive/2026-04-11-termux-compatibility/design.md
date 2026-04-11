# Design: termux-compatibility

## Technical Approach
Implement a `system.PathResolver` that dynamically adjusts filesystem paths based on the detected environment. In Termux, it prepends the `$PREFIX` variable. This approach avoids hardcoded `if` blocks throughout the codebase and ensures that the core logic remains platform-agnostic.

## Architecture Decisions

### Decision: Interface-based Path Resolution
**Choice**: Create a `PathResolver` interface and a `DefaultResolver` vs `TermuxResolver`.
**Alternatives considered**: Global `if` checks in every file using paths.
**Rationale**: Highly maintainable and testable. We can inject a mocked resolver in tests to verify Termux logic on any host OS.

### Decision: Shell-based PATH Persistence
**Choice**: Direct modification of `~/.bashrc` and `~/.zshrc`.
**Alternatives considered**: Using `termux-setup-storage` or system-wide profile changes.
**Rationale**: Standard practice in Termux. It avoids requiring root/su permissions and ensures the PATH survives terminal restarts.

## Data Flow
`System Detection` -> `Profile Assignment` -> `Resolver Injection` -> `Path Resolution`

```
[Main Context] -> [Detect()] -> [PlatformProfile (Termux)]
                      |
                      v
[Resolver] <--- [Injected into components]
    |
    +--> [TermuxResolver]: Prepend $PREFIX/bin
    +--> [DefaultResolver]: Pass-through (/usr/bin)
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/system/resolver.go` | Create | Define `PathResolver` interface and implementations. |
| `internal/system/detect.go` | Modify | Add Termux detection and resolver initialization. |
| `internal/system/path.go` | Modify | Update `AddToUserPath` to handle Termux shell config files. |
| `internal/update/upgrade/strategy.go` | Modify | Include `-extldflags=-pie` for Android builds. |
| `internal/installcmd/resolver.go` | Modify | Replace hardcoded `/usr/bin` with `Resolver.Resolve()`. |

## Interfaces / Contracts

```go
type PathResolver interface {
    Resolve(path string) string
}

type TermuxResolver struct {
    Prefix string
}

func (r *TermuxResolver) Resolve(path string) string {
    // e.g., /usr/bin/bash -> $PREFIX/bin/bash
    return filepath.Join(r.Prefix, strings.TrimPrefix(path, "/usr"))
}
```

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | `PathResolver` | Test both `Default` and `Termux` resolvers with various path inputs. |
| Unit | `detectFromInputs` | Mock `os-release` and env vars to verify `Termux` distro detection. |
| Integration | `AddToUserPath` | Mock filesystem to verify `.bashrc` modification in Termux mode. |

## Migration / Rollout
No migration required for existing users on Windows/Linux. New users on Termux will get the correct environment detection automatically.

## Open Questions
- [ ] Should we automatically run `termux-setup-storage` if access to `/sdcard` is needed? (Currently out of scope).

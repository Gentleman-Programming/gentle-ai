# SDD Design: Human Gates Programáticos

## Modificaciones a Structs

En `internal/sddstatus/status.go`:

```go
type ArtifactPaths struct {
    // ...
    Gate1Scope          []string `json:"gate1Scope"`
    Gate2Technical      []string `json:"gate2Technical"`
    Gate3Implementation []string `json:"gate3Implementation"`
}

type Dependencies struct {
    // ...
    Gate1Scope          DependencyState `json:"gate1Scope"`
    Gate2Technical      DependencyState `json:"gate2Technical"`
    Gate3Implementation DependencyState `json:"gate3Implementation"`
}
```

## Lógica de Dependencias (`resolveDependencies`)
```go
	dependencies.Gate1Scope = DependencyBlocked
	if dependencies.Proposal == DependencyAllDone {
		if artifacts["gate1Scope"] == ArtifactDone {
			dependencies.Gate1Scope = DependencyAllDone
			dependencies.Specs = DependencyReady
			dependencies.Design = DependencyReady
		} else {
			dependencies.Gate1Scope = DependencyReady
		}
	}
// Mismo patrón para Gate2 y Gate3, requiriendo que Propsal/Specs/Design/Tasks se integren de acuerdo a las ramas.
```

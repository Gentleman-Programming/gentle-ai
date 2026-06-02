# Go Testing Examples

## Table-Driven Test

```go
func TestProcessInput(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {name: "valid input", input: "hello", want: "HELLO"},
        {name: "empty input", input: "", wantErr: true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ProcessInput(tt.input)
            if (err != nil) != tt.wantErr {
                t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
            }
            if got != tt.want {
                t.Fatalf("got %q, want %q", got, tt.want)
            }
        })
    }
}
```

## Bubbletea State Transition

```go
func TestModelUpdateEnter(t *testing.T) {
    m := NewModel()
    next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
    got := next.(Model)
    if got.Screen != ScreenMainMenu {
        t.Fatalf("screen = %v, want %v", got.Screen, ScreenMainMenu)
    }
}
```

## Teatest Flow

```go
func TestInteractiveFlow(t *testing.T) {
    tm := teatest.NewTestModel(t, NewModel())
    tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
    tm.WaitFinished(t, teatest.WithDuration(time.Second))
    final := tm.FinalModel(t).(Model)
    if final.Screen != ExpectedScreen {
        t.Fatalf("screen = %v, want %v", final.Screen, ExpectedScreen)
    }
}
```

## Golden File Pattern

```go
var update = flag.Bool("update", false, "update golden files")

func assertGolden(t *testing.T, path string, got string) {
    t.Helper()
    if *update {
        if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
            t.Fatal(err)
        }
    }
    want, err := os.ReadFile(path)
    if err != nil {
        t.Fatal(err)
    }
    if got != string(want) {
        t.Fatalf("golden mismatch for %s", path)
    }
}
```

## Commands

```bash
go test ./...
go test -v ./internal/tui/...
go test -run TestNavigation ./internal/tui
go test -cover ./...
go test ./internal/components -update
go test -short ./...
```

package telemetry

import (
	"os"
	"strings"
	"testing"
)

// Guard the absence of runtime background/persistence mechanisms, complementing
// behavioral filesystem and HTTP tests. Context deadlines are not retry timers.
func TestRuntimeHasNoSchedulerOrMetricPersistence(t *testing.T) {
	for _, path := range []string{"runtime.go", "runtime_send.go", "runtime_flush.go", "runtime_schedule.go", "runtime_state.go"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"os.WriteFile(", "os.Create(", "os.Mkdir", "os.Remove", "filemerge.", "lockRuntime(", "exec.Command", "time.NewTicker(", "time.AfterFunc(", "time.Sleep(", "EnsureState(", "storeRuntime(", "ScheduleRuntimeFlush(", "SpawnDetachedRuntimeFlush("} {
			if strings.Contains(string(raw), forbidden) {
				t.Errorf("%s contains forbidden mechanism %s", path, forbidden)
			}
		}
	}
}

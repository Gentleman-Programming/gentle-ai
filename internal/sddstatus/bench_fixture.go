//go:build bench_fixture

package sddstatus

import "os"

func init() {
	afterCompactPreVerifyAuthorityInitialRead = func() {
		path := os.Getenv("GENTLE_AI_BENCH_MUTATE_RECEIPT")
		if path == "" {
			return
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return
		}
		_ = os.WriteFile(path, append(payload, '\n'), 0o644)
	}
}

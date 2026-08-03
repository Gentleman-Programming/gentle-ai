//go:build bench_fixture

package sddstatus

import (
	"fmt"
	"os"
)

func init() {
	afterCompactPreVerifyAuthorityInitialRead = func() error {
		path := os.Getenv("GENTLE_AI_BENCH_MUTATE_RECEIPT")
		if path == "" {
			return nil
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read receipt %q: %w", path, err)
		}
		if err := os.WriteFile(path, append(payload, '\n'), 0o644); err != nil {
			return fmt.Errorf("write receipt %q: %w", path, err)
		}
		return nil
	}
}

//go:build !bench_fixture

package app

import "io"

func runBenchModelPickerCommand(_ []string, _ io.Writer) (bool, error) {
	return false, nil
}

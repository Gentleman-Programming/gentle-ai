package system

import "os"

// EngramBinEnv names a caller-supplied Engram executable (an absolute path).
// When set, the install uses that binary instead of acquiring Engram, and
// dependency detection runs no tool: see DetectDependencies.
const EngramBinEnv = "GENTLE_AI_ENGRAM_BIN"

func callerSuppliesEngram() bool {
	return os.Getenv(EngramBinEnv) != ""
}

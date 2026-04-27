package engram

import "os"

// DataDirEnvVar is the environment variable that controls where Engram stores
// its persistent SQLite database and related files.
const DataDirEnvVar = "ENGRAM_DATA_DIR"

// getEngramDataDirEnv returns the current ENGRAM_DATA_DIR env var value.
func getEngramDataDirEnv() string {
	return os.Getenv(DataDirEnvVar)
}

// setEngramDataDirEnv sets the ENGRAM_DATA_DIR env var.
func setEngramDataDirEnv(dir string) error {
	return os.Setenv(DataDirEnvVar, dir)
}

// unsetEngramDataDirEnv unsets the ENGRAM_DATA_DIR env var.
func unsetEngramDataDirEnv() error {
	return os.Unsetenv(DataDirEnvVar)
}

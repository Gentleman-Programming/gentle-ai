//go:build !windows

package dashboard

// getLogicalDrives retorna la raíz del sistema de ficheros en entornos Unix.
func getLogicalDrives() []string {
	return []string{"/"}
}

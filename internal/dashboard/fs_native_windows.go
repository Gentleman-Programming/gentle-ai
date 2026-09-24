//go:build windows

package dashboard

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

func pickFolderNative(ctx context.Context, initialPath string) (string, error) {
	psScript := `Add-Type -AssemblyName System.Windows.Forms; $d = New-Object System.Windows.Forms.FolderBrowserDialog; $d.Description = 'Selecciona una carpeta para Axiom'; $d.ShowNewFolderButton = $true;`
	if initialPath != "" {
		escaped := strings.ReplaceAll(initialPath, `'`, `''`)
		psScript += ` $d.SelectedPath = '` + escaped + `';`
	}
	psScript += ` if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { Write-Output $d.SelectedPath }`

	cmdCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

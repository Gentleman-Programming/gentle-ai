package reviewerprovider

import "github.com/gentleman-programming/gentle-ai/v4/internal/system"

// isWindowsBatchFile reports whether path names a Windows batch script rather
// than a native executable. Delegated to internal/system.IsWindowsBatchFile.
func isWindowsBatchFile(path string) bool {
	return system.IsWindowsBatchFile(path)
}

// windowsBatchCommandLine builds the single command-line string Windows must
// hand to cmd.exe to launch a .bat/.cmd target whose path contains a space.
// Delegated to internal/system.WindowsBatchCommandLine.
func windowsBatchCommandLine(binary string, args []string) string {
	return system.WindowsBatchCommandLine(binary, args)
}

// quoteWindowsArgument applies CommandLineToArgvW-compatible escaping.
// Delegated to internal/system.QuoteWindowsArgument.
func quoteWindowsArgument(arg string) string {
	return system.QuoteWindowsArgument(arg)
}

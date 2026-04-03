package upgrade

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/system"
	"github.com/gentleman-programming/gentle-ai/internal/update"
)

// httpClient is the HTTP client used for asset downloads.
// Package-level var for testability.
var httpClient = &http.Client{Timeout: 5 * time.Minute}

// lookPathFn resolves the binary path. Package-level var for testability.
var lookPathFn = exec.LookPath

// Download downloads the GitHub release binary for the given tool and replaces
// the installed binary atomically.
//
// This function is not called on Windows — callers (strategy.go) gate it via
// platform check and return a manual fallback error instead.
func Download(ctx context.Context, r update.UpdateResult, profile system.PlatformProfile) error {
	if profile.OS == "windows" {
		hint := r.UpdateHint
		if hint == "" {
			hint = fmt.Sprintf("Download from https://github.com/%s/%s/releases", r.Tool.Owner, r.Tool.Repo)
		}
		return fmt.Errorf("upgrade %q on Windows requires manual update — %s", r.Tool.Name, hint)
	}

	// Resolve the current binary path from PATH.
	binaryPath, err := lookPathFn(r.Tool.Name)
	if err != nil {
		return fmt.Errorf("locate %q binary: %w", r.Tool.Name, err)
	}

	// Build the asset URL.
	assetURL := resolveAssetURL(r.Tool.Owner, r.Tool.Repo, r.LatestVersion, profile.OS, runtime.GOARCH)

	// Download to a temp file.
	tmpPath := binaryPath + ".new"
	if err := downloadBinary(ctx, assetURL, r.Tool.Name, tmpPath); err != nil {
		return fmt.Errorf("download %s: %w", r.Tool.Name, err)
	}

	// Atomic replace.
	if err := atomicReplace(tmpPath, binaryPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace %q: %w", binaryPath, err)
	}

	return nil
}

// resolveAssetURL constructs the GitHub Releases asset download URL.
// Convention: Gentleman-Programming repos use the goreleaser default naming:
//
//	{repo}_{version}_{os}_{arch}.tar.gz
func resolveAssetURL(owner, repo, version, goos, goarch string) string {
	filename := fmt.Sprintf("%s_%s_%s_%s.tar.gz", repo, version, goos, goarch)
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/v%s/%s",
		owner, repo, version, filename)
}

// downloadBinary fetches the asset at url, extracts the binary named binaryName
// from the .tar.gz, and writes it to outPath with executable permissions.
func downloadBinary(ctx context.Context, url string, binaryName string, outPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}

	return extractBinaryFromTarGz(resp.Body, binaryName, outPath)
}

// extractBinaryFromTarGz reads a .tar.gz stream and extracts the first file
// whose base name matches binaryName, writing it to outPath.
func extractBinaryFromTarGz(r io.Reader, binaryName string, outPath string) error {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("open gzip: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}

		// Match by base name (handles subdirectory layouts like tool_1.0_os_arch/tool).
		// Only accept regular files — skip symlinks, hardlinks, and special files.
		if filepath.Base(hdr.Name) == binaryName &&
			(hdr.Typeflag == tar.TypeReg || hdr.Typeflag == tar.TypeRegA) {
			if err := writeExecutable(tr, outPath); err != nil {
				return err
			}
			return nil
		}
	}

	return fmt.Errorf("binary %q not found in archive", binaryName)
}

// writeExecutable writes the content from r to outPath with executable permissions.
func writeExecutable(r io.Reader, outPath string) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("create %s: %w", outPath, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("write %s: %w", outPath, err)
	}

	return nil
}

// atomicReplace moves src to dst atomically using os.Rename.
// This is safe on Unix (same-filesystem rename) but NOT safe on Windows
// when the binary is running. The caller must guard against Windows before calling.
func atomicReplace(src, dst string) error {
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", src, dst, err)
	}
	return nil
}

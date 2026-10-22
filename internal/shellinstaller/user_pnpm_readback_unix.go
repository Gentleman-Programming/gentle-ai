//go:build linux && amd64

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// NativePNPMInstallation identifies checked targets relative to the private root.
// Files contains physical stamps plus streaming hashes, not provenance receipts.
type NativePNPMInstallation struct {
	Node, PiCLI, ShellCLI, PeerCLI, SDKRoot string
	Files                                   map[string]string
}

// ReadNativePNPMInstallation reads the original producer's immutable PNPM output.
// The caller retains source/worker custody. This neither executes packages nor
// validates the full dependency closure, writes a manifest, or publishes files.
func ReadNativePNPMInstallation(ctx context.Context, root string, location NativeInstallationLocation) (NativePNPMInstallation, error) {
	result := NativePNPMInstallation{Files: map[string]string{}}
	fail := func(err error) (NativePNPMInstallation, error) {
		return NativePNPMInstallation{}, privateError("readback", err)
	}
	home := filepath.Join(root, "pnpm")
	if err := nativeRuntimeError(ctx, nil); err != nil {
		return fail(err)
	}
	if _, err := privateDirectory(root); err != nil {
		return fail(err)
	}
	if location.PNPMHome != home || location.GlobalBin != filepath.Join(home, "bin") ||
		!userNativeSemver(location.ShellVersion) || !userNativeSemver(location.PiVersion) {
		return fail(nil)
	}
	if err := privateNativeDirectory(home); err != nil {
		return fail(err)
	}
	record := func(path string) (string, error) {
		physical, err := userPNPMResolve(home, path)
		if err != nil {
			return "", err
		}
		stamp, err := privateNativeFile(ctx, physical, 0, -1, "")
		rel, _ := filepath.Rel(root, physical)
		result.Files[rel] = stamp
		return rel, err
	}
	packageEntry := func(path, name, version, bin, entry string) (string, error) {
		metadata, err := userPNPMResolve(home, filepath.Join(path, "package.json"))
		if err != nil {
			return "", err
		}
		data, stamp, err := userPNPMSmallFile(ctx, metadata)
		var pkg struct {
			Name, Version string
			Bin           map[string]string
		}
		if err != nil || json.Unmarshal(data, &pkg) != nil || pkg.Name != name || pkg.Bin[bin] != entry {
			return "", privateError("readback", err)
		}
		// An empty expected version admits any canonical stock release: the
		// independent CLI version is read from the producer metadata, never frozen.
		if version == "" {
			if !userNativeSemver(pkg.Version) {
				return "", privateError("readback", nil)
			}
		} else if pkg.Version != version {
			return "", privateError("readback", nil)
		}
		rel, _ := filepath.Rel(root, metadata)
		result.Files[rel] = stamp
		return record(filepath.Join(filepath.Dir(metadata), entry))
	}
	for _, spec := range []struct {
		bin, name, version, entry string
		target                    *string
	}{
		{"pi", "@earendil-works/pi-coding-agent", "", "dist/bundle/cli.js", &result.PiCLI},
		{"gentle-shell", "gentle-pi", location.ShellVersion, "bin/gentle-shell.mjs", &result.ShellCLI},
	} {
		shim := filepath.Join(location.GlobalBin, spec.bin)
		body, stamp, err := userPNPMSmallFile(ctx, shim)
		marker := "\n# cmd-shim-target="
		parts := strings.Split(string(body), marker)
		if err != nil || len(parts) != 2 {
			return fail(err)
		}
		target := strings.TrimSuffix(parts[1], "\n")
		if !privateHierarchyPath(target) || !strings.HasPrefix(target, home+"/global/v11/") || !strings.HasSuffix(target, "/"+spec.entry) {
			return fail(nil)
		}
		rel, _ := filepath.Rel(location.GlobalBin, target)
		if strings.ContainsAny(rel, "\"$`\\\r\n") || string(body) != fmt.Sprintf(userPNPMGlobalShim, rel, rel, target) {
			return fail(nil)
		}
		result.Files[filepath.Join("pnpm/bin", spec.bin)] = stamp
		*spec.target, err = packageEntry(strings.TrimSuffix(target, "/"+spec.entry), spec.name, spec.version, spec.bin, spec.entry)
		if err != nil {
			return fail(err)
		}
	}
	if filepath.Join(root, result.ShellCLI) != filepath.Join(location.PackageRoot, "bin/gentle-shell.mjs") {
		return fail(nil)
	}
	node, err := userPNPMResolve(home, filepath.Join(location.GlobalBin, "node"))
	if err != nil {
		return fail(err)
	}
	result.Node, err = packageEntry(filepath.Dir(filepath.Dir(node)), "node", "24.21.0", "node", "bin/node")
	if err != nil || filepath.Join(root, result.Node) != node {
		return fail(err)
	}
	if info, err := os.Stat(node); err != nil || info.Mode()&0100 == 0 {
		return fail(err)
	}
	result.PeerCLI, err = packageEntry(filepath.Join(filepath.Dir(location.PackageRoot), "@earendil-works/pi-coding-agent"), "@earendil-works/pi-coding-agent", location.PiVersion, "pi", "dist/bundle/cli.js")
	if err != nil {
		return fail(err)
	}
	sdk := filepath.Join(location.PackageRoot, ".gentle-ai")
	if err := userNativeReadback(ctx, sdk); err != nil {
		return fail(err)
	}
	result.SDKRoot, _ = filepath.Rel(root, sdk)
	return result, nil
}

func userPNPMResolve(home, path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || !strings.HasPrefix(resolved, home+"/") {
		return "", privateError("readback", err)
	}
	for hops := 0; hops < 40; hops++ {
		if !privateHierarchyPath(path) || !strings.HasPrefix(path, home+"/") {
			break
		}
		parts := strings.Split(strings.TrimPrefix(path, home+"/"), "/")
		current, redirected := home, false
		for i, part := range parts {
			current = filepath.Join(current, part)
			info, err := os.Lstat(current)
			if err != nil || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) || info.Sys().(*syscall.Stat_t).Gid != uint32(os.Getgid()) {
				return "", privateError("readback", err)
			}
			if info.Mode()&os.ModeSymlink != 0 {
				target, err := os.Readlink(current)
				if err != nil {
					return "", err
				}
				if !filepath.IsAbs(target) {
					target = filepath.Join(filepath.Dir(current), target)
				}
				path, redirected = filepath.Join(target, filepath.Join(parts[i+1:]...)), true
				break
			}
			if info.IsDir() {
				if err := privateNativeDirectory(current); err != nil {
					return "", err
				}
			} else if i != len(parts)-1 || !info.Mode().IsRegular() {
				return "", privateError("readback", nil)
			}
		}
		if !redirected {
			if current != resolved {
				return "", privateError("readback", nil)
			}
			return current, nil
		}
	}
	return "", privateError("readback", nil)
}

func userPNPMSmallFile(ctx context.Context, path string) ([]byte, string, error) {
	if _, err := privatePhysical(path); err != nil {
		return nil, "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	data, err := userPNPMReadSmall(file)
	if err != nil {
		return nil, "", err
	}
	stamp, err := privateNativeFile(ctx, path, 0, int64(len(data)), fmt.Sprintf("%x", sha256.Sum256(data)))
	return data, stamp, err
}

// Own the bounded reader through close; failed close never yields usable DATA.
func userPNPMReadSmall(reader io.ReadCloser) ([]byte, error) {
	data, readErr := io.ReadAll(io.LimitReader(reader, 65537))
	err := errors.Join(readErr, reader.Close())
	if err != nil || len(data) > 65536 {
		return nil, privateError("readback", err)
	}
	return data, nil
}

const userPNPMGlobalShim = `#!/bin/sh
basedir=$(dirname "$(echo "$0" | sed -e 's,\\,/,g')")

case ` + "`uname`" + ` in
    *CYGWIN*|*MINGW*|*MSYS*)
        if command -v cygpath > /dev/null 2>&1; then
            basedir=` + "`cygpath -w \"$basedir\"`" + `
        fi
    ;;
esac

if [ -x "$basedir/node" ]; then
  exec "$basedir/node"  "$basedir/%s" "$@"
else
  exec node  "$basedir/%s" "$@"
fi
# cmd-shim-target=%s
`

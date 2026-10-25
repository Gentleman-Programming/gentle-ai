//go:build windows

package shellinstaller

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

func privateError(kind string, err error) error {
	if err == nil {
		return errors.New(kind)
	}
	return fmt.Errorf("%s: %w", kind, err)
}

// CodeGraph 1.6.2 is pinned as the framework package plus its win32-x64
// payload. The bundled node.exe is deliberately omitted: the installer's own
// authenticated Node is the only runtime we publish.
var userWindowsCodeGraphSources = []struct {
	name, url, sri, sha, archive string
	size, unpacked               int64
	entries                      int
	skipNode                     bool
}{
	{
		name: "codegraph", url: "https://registry.npmjs.org/@colbymchenry/codegraph/-/codegraph-1.6.2.tgz",
		sri: "sha512-hODH/mGnCf3IUMs6rzktv/zbecJIKJfdZAYHZ9mqecb0G7ZkHaA8CZafDVnswANFEL1YFcG+lfAwc5SIXzUH6Q==",
		sha: "80cb635659cdb21f25bb882331bc82e0180d42e80550186712e12ab0e04082ea", archive: "codegraph.tgz",
		size: 424466, unpacked: 1208076, entries: 265,
	},
	{
		name: "codegraph-win32-x64", url: "https://registry.npmjs.org/@colbymchenry/codegraph-win32-x64/-/codegraph-win32-x64-1.6.2.tgz",
		sri: "sha512-3psrtVSdZIoqHLmdeL1b9OCPeKtuPKmVjg2/3XcOLaBwu0osMRwYFHG6uN1iSKQOKfre8jL+t2KXV8fYl1VxAQ==",
		sha: "2d971d24d77e8de43455a4210b561d6c60ac845d92229458198f294be824de14", archive: "codegraph-win32-x64.tgz",
		size: 54584451, unpacked: 265095603, entries: 1322, skipNode: true,
	},
}

func userWindowsCodeGraphProvision(ctx context.Context, root string, profile *UserExperience) error {
	if profile == nil || profile.CodeGraph != "on" {
		return nil
	}
	if ctx == nil || ctx.Err() != nil || !filepath.IsAbs(root) {
		return privateError("refused", errors.New("Windows CodeGraph requires a live private stage"))
	}
	if _, err := userWindowsIdentity(root, true); err != nil {
		return err
	}
	archives := filepath.Join(root, "runtime", "archives")
	for _, source := range userWindowsCodeGraphSources {
		artifact := userWindowsArtifact{
			URL: source.url, SHA: source.sha, Archive: source.archive,
			Prefix: "", Bound: 64 << 20, Size: source.size,
		}
		fmt.Fprintf(os.Stderr, "Windows install phase: codegraph-download %s\n", source.name)
		data, err := userWindowsAcquire(ctx, artifact, filepath.Join(archives, source.archive))
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Windows install phase: codegraph-extract %s\n", source.name)
		if err := userWindowsCodeGraphExtract(ctx, root, source, data); err != nil {
			return err
		}
		if err := os.Remove(filepath.Join(archives, source.archive)); err != nil {
			return err
		}
	}
	return userWindowsCodeGraphCommand(ctx, root)
}

func userWindowsCodeGraphExtract(ctx context.Context, root string, source struct {
	name, url, sri, sha, archive string
	size, unpacked               int64
	entries                      int
	skipNode                     bool
}, data []byte) error {
	if int64(len(data)) != source.size {
		return privateError("source", errors.New("CodeGraph archive size differs"))
	}
	sum := sha512.Sum512(data)
	if "sha512-"+base64.StdEncoding.EncodeToString(sum[:]) != source.sri {
		return privateError("source", errors.New("CodeGraph archive integrity differs"))
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gz.Close()
	reader := tar.NewReader(io.LimitReader(gz, source.unpacked+int64(source.entries)*1024+1025))
	seen := map[string]bool{}
	var total int64
	var metadata []byte
	files := map[string][]byte{}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil || header.Typeflag != tar.TypeReg || !strings.HasPrefix(header.Name, "package/") || path.Clean(header.Name) != header.Name || strings.Contains(header.Name, "\\") || seen[header.Name] || len(seen) >= source.entries {
			return privateError("source", errors.New("CodeGraph member refused"))
		}
		seen[header.Name] = true
		total += header.Size
		if source.skipNode && header.Name == "package/node.exe" {
			continue
		}
		if header.Size < 0 || header.Size > 64<<20 {
			return privateError("source", errors.New("CodeGraph member exceeds 64MiB"))
		}
		body, err := io.ReadAll(reader)
		if err != nil || int64(len(body)) != header.Size {
			return privateError("source", errors.New("CodeGraph member bytes differ"))
		}
		relative := "runtime/codegraph/node_modules/@colbymchenry/" + source.name + "/" + strings.TrimPrefix(header.Name, "package/")
		files[relative] = body
		if header.Name == "package/package.json" {
			metadata = body
		}
	}
	if int64(len(seen)) != int64(source.entries) || total != source.unpacked {
		return privateError("source", errors.New("CodeGraph archive shape differs"))
	}
	var pkg struct{ Name, Version string }
	if json.Unmarshal(metadata, &pkg) != nil || pkg.Name != "@colbymchenry/"+source.name || pkg.Version != "1.6.2" {
		return privateError("source", errors.New("CodeGraph package identity differs"))
	}
	dirs := map[string]bool{}
	for name := range files {
		for dir := filepath.Dir(name); dir != "."; dir = filepath.Dir(dir) {
			dirs[dir] = true
		}
	}
	names := make([]string, 0, len(dirs))
	for name := range dirs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(full, 0700); err != nil {
			return err
		}
		if err := userWindowsPrivate(full); err != nil {
			return err
		}
	}
	for relative, body := range files {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := userWindowsWrite(filepath.Join(root, filepath.FromSlash(relative)), body); err != nil {
			return err
		}
	}
	return nil
}

func userWindowsCodeGraphCommand(ctx context.Context, root string) error {
	if ctx == nil || ctx.Err() != nil {
		return privateError("canceled", context.Canceled)
	}
	bin := filepath.Join(root, "runtime", "codegraph", "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		return err
	}
	if err := userWindowsPrivate(bin); err != nil {
		return err
	}
	command := strings.Join([]string{
		`@echo off`,
		`"%~dp0..\..\node\node.exe" "%~dp0..\node_modules\@colbymchenry\codegraph-win32-x64\lib\dist\bin\codegraph.js" %*`,
	}, "\r\n") + "\r\n"
	return userWindowsWrite(filepath.Join(bin, "codegraph.cmd"), []byte(command))
}

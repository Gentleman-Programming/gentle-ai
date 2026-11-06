package shellinstaller

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

// Reuse the existing ZIP preflight before any platform-owned extraction.
// Native Node/Go exceptions remain selected only by their existing recipes.
func userSnapshotZIP(ctx context.Context, data []byte, artifact userSnapshotArtifact) (*zip.Reader, error) {
	if ctx == nil {
		return nil, context.Canceled
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if int64(len(data)) > artifact.Bound || fmt.Sprintf("%x", sha256.Sum256(data)) != artifact.SHA {
		return nil, errors.New("Windows archive authority differs")
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(reader.File) > 32768 {
		return nil, errors.Join(err, errors.New("Windows archive entry bound"))
	}
	seen := map[string]bool{}
	var total uint64
	for _, file := range reader.File {
		name := strings.TrimSuffix(file.Name, "/")
		key := strings.ToLower(name)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if name == "" || (name == artifact.Prefix && !file.FileInfo().IsDir()) || seen[key] || file.Mode()&os.ModeSymlink != 0 || strings.ContainsAny(name, "\\:\x00\r\n") || (name != artifact.Prefix && !strings.HasPrefix(name, artifact.Prefix+"/")) || (!file.Mode().IsRegular() && !file.FileInfo().IsDir()) {
			return nil, errors.New("Windows archive path/type/duplicate refused")
		}
		seen[key] = true
		for _, part := range strings.Split(name, "/") {
			base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
			if part == "" || part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || (len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9') {
				return nil, errors.New("Windows archive noncanonical component")
			}
		}
		memberBound := uint64(32 << 20)
		if artifact.Archive == "node.zip" || artifact.Archive == "go.zip" {
			memberBound = 128 << 20 // Authenticated Node/compiler executables.
		}
		if file.UncompressedSize64 > memberBound {
			return nil, errors.New("Windows archive member byte bound")
		}
		total += file.UncompressedSize64
		if total > 1<<30 {
			return nil, errors.New("Windows archive aggregate byte bound")
		}
	}
	return reader, nil
}

func userMainSourceFiles(ctx context.Context, data []byte, artifact userSnapshotArtifact) (map[string]*zip.File, error) {
	canonical, err := userMainDescriptor(strings.TrimPrefix(artifact.Prefix, "gentle-shell-"), artifact.SHA)
	if err != nil || canonical != artifact {
		return nil, errors.Join(err, errors.New("Main source descriptor differs"))
	}
	reader, err := userSnapshotZIP(ctx, data, artifact)
	if err != nil {
		return nil, err
	}
	files := map[string]*zip.File{}
	for _, file := range reader.File {
		name := strings.TrimSuffix(file.Name, "/")
		if name == artifact.Prefix {
			continue
		}
		relative := strings.TrimPrefix(name, artifact.Prefix+"/")
		first := strings.ToLower(strings.Split(relative, "/")[0])
		if first == "node_modules" || first == ".gentle-ai" {
			return nil, errors.New("Main archive may not replace native/dependency custody")
		}
		files[relative] = file
		for parent := path.Dir(relative); parent != "."; parent = path.Dir(parent) {
			if _, exists := files[parent]; !exists {
				files[parent] = nil // Implicit archive directory.
			}
		}
	}
	// Include implicit directories in the exact set and refuse impossible
	// hierarchies before the platform consumer can publish any source file.
	canonicalNames := map[string]bool{}
	for name := range files {
		key := strings.ToLower(name)
		if canonicalNames[key] {
			return nil, errors.New("Main source directory alias refused")
		}
		canonicalNames[key] = true
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if file := files[parent]; file != nil && !file.FileInfo().IsDir() {
				return nil, errors.New("Main source parent is not a directory")
			}
		}
	}
	pkg := files["package.json"]
	if pkg == nil || !pkg.Mode().IsRegular() {
		return nil, errors.New("Main archive package.json missing or not regular")
	}
	stream, err := pkg.Open()
	if err != nil {
		return nil, err
	}
	content, readErr := io.ReadAll(io.LimitReader(stream, int64(pkg.UncompressedSize64)+1))
	if err := errors.Join(readErr, stream.Close(), ctx.Err()); err != nil || uint64(len(content)) != pkg.UncompressedSize64 {
		return nil, errors.Join(err, errors.New("Main package bytes differ"))
	}
	return files, nil
}

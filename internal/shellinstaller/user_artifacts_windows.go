//go:build windows

package shellinstaller

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type userWindowsArtifact struct {
	URL, SHA, Archive, Prefix string
	Bound, Size               int64
}

// Descriptor construction alone does not authenticate or resolve Main source.
func userWindowsMainArtifact(commit, archiveSHA string) (userWindowsArtifact, error) {
	if !userWindowsLowerHex(commit, 40) || !userWindowsLowerHex(archiveSHA, 64) {
		return userWindowsArtifact{}, errors.New("invalid frozen Gentle Shell Main commit/archive digest")
	}
	return userWindowsArtifact{URL: "https://codeload.github.com/Gentleman-Programming/gentle-shell/zip/" + commit, SHA: archiveSHA, Archive: "main.zip", Prefix: "gentle-shell-" + commit, Bound: 32 << 20}, nil
}

func userWindowsLowerHex(value string, length int) bool {
	_, err := hex.DecodeString(value)
	return len(value) == length && err == nil && value == strings.ToLower(value)
}

// One fixed-publisher ref lookup; no redirects, proxy, credentials or retries.
func userWindowsMainSnapshot(ctx context.Context) (userWindowsArtifact, []byte, error) {
	if ctx == nil || ctx.Err() != nil {
		return userWindowsArtifact{}, nil, errors.New("Main acquisition canceled before requests")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy, transport.DisableCompression = nil, true
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("Main publisher redirect refused")
	}}
	get := func(url string, bound int64) ([]byte, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept-Encoding", "identity")
		req.Header.Set("User-Agent", "gentle-shell-windows-installer")
		response, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK || response.ContentLength > bound || (response.Header.Get("Content-Encoding") != "" && response.Header.Get("Content-Encoding") != "identity") {
			return nil, errors.New("Main publisher status/encoding/bound refused")
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, bound+1))
		if err != nil || int64(len(data)) > bound {
			return nil, errors.Join(err, errors.New("Main publisher response bound"))
		}
		return data, nil
	}
	metadata, err := get("https://api.github.com/repos/Gentleman-Programming/gentle-shell/commits/main", 256<<10)
	var record struct {
		SHA string `json:"sha"`
	}
	if err != nil {
		return userWindowsArtifact{}, nil, err
	}
	if err := json.Unmarshal(metadata, &record); err != nil || !userWindowsLowerHex(record.SHA, 40) {
		return userWindowsArtifact{}, nil, errors.Join(err, errors.New("canonical Main ref identity refused"))
	}
	data, err := get("https://codeload.github.com/Gentleman-Programming/gentle-shell/zip/"+record.SHA, 32<<20)
	if err != nil {
		return userWindowsArtifact{}, nil, err
	}
	artifact, err := userWindowsMainArtifact(record.SHA, userWindowsSHA(data))
	return artifact, data, err
}

func userWindowsMainFiles(ctx context.Context, data []byte, artifact userWindowsArtifact) (map[string]*zip.File, error) {
	if _, err := userWindowsZIP(ctx, data, artifact, "", artifact.Prefix+"/package.json"); err != nil {
		return nil, err
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
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
		for parent := filepath.ToSlash(filepath.Dir(relative)); parent != "."; parent = filepath.ToSlash(filepath.Dir(parent)) {
			if _, exists := files[parent]; !exists {
				files[parent] = nil // Implicit archive directory.
			}
		}
	}
	return files, nil
}

func userWindowsCompareMain(ctx context.Context, data []byte, artifact userWindowsArtifact, destination string) error {
	files, err := userWindowsMainFiles(ctx, data, artifact)
	if err != nil {
		return err
	}
	err = filepath.WalkDir(destination, func(path string, entry os.DirEntry, cause error) error {
		if cause != nil || ctx.Err() != nil {
			return errors.Join(cause, ctx.Err())
		}
		relative, err := filepath.Rel(destination, path)
		if err != nil || relative == "." {
			return err
		}
		relative = filepath.ToSlash(relative)
		if _, err := userWindowsIdentity(path, true); err != nil {
			return err
		}
		if (relative == "node_modules" || relative == ".gentle-ai") && entry.IsDir() {
			return filepath.SkipDir // Verified separately against npm and SumDB custody.
		}
		file, exists := files[relative]
		if !exists || entry.IsDir() != (file == nil || file.FileInfo().IsDir()) {
			return errors.New("Main source exact set/type differs")
		}
		delete(files, relative)
		if entry.IsDir() {
			return nil
		}
		stream, err := file.Open()
		if err != nil {
			return err
		}
		original, readErr := io.ReadAll(io.LimitReader(stream, int64(file.UncompressedSize64)+1))
		closeErr := stream.Close()
		actual, actualErr := userWindowsRead(path, int64(file.UncompressedSize64))
		if cause := errors.Join(readErr, closeErr, actualErr); cause != nil || !bytes.Equal(original, actual) {
			return errors.Join(cause, errors.New("Main source bytes differ from retained archive"))
		}
		return nil
	})
	if err != nil || len(files) != 0 {
		return errors.Join(err, errors.New("Main source missing/extra members"))
	}
	return nil
}

func userWindowsMainRead(root string, manifest userWindowsManifest) (userWindowsArtifact, error) {
	artifact, err := userWindowsMainArtifact(manifest.MainCommit, manifest.MainArchiveSHA)
	if err != nil {
		return userWindowsArtifact{}, err
	}
	data, err := userWindowsRead(filepath.Join(root, "main.json"), 4096)
	canonical, _ := json.Marshal(artifact)
	if err != nil || string(data) != string(canonical) {
		return userWindowsArtifact{}, errors.Join(err, errors.New("retained Main descriptor differs"))
	}
	return artifact, nil
}

var userWindowsArtifacts = []userWindowsArtifact{
	{"https://nodejs.org/dist/v24.18.0/node-v24.18.0-win-x64.zip", "0ae68406b42d7725661da979b1403ec9926da205c6770827f33aac9d8f26e821", "node.zip", "node-v24.18.0-win-x64", 64 << 20, 0},
	{"https://go.dev/dl/go1.27.1.windows-amd64.zip", "a3911b5e0e1b1053f25ed0675f4c1c6aad1e2bfcf253df2b9be4caabd2edd95d", "go.zip", "go", 128 << 20, 78931360},
	{"https://github.com/sharkdp/fd/releases/download/v10.5.0/fd-v10.5.0-x86_64-pc-windows-msvc.zip", "a227701b8551c35a9931d9f6da75503cf86d88e182d71fb849a70864c5d57cd7", "fd.zip", "fd-v10.5.0-x86_64-pc-windows-msvc", 32 << 20, 1535063},
	{"https://github.com/BurntSushi/ripgrep/releases/download/15.2.0/ripgrep-15.2.0-x86_64-pc-windows-msvc.zip", "71b2fef860abe467217a538ff31de02f5258807c0129f771846f87bd029aafc5", "rg.zip", "ripgrep-15.2.0-x86_64-pc-windows-msvc", 32 << 20, 1789611},
}

func userWindowsAcquire(ctx context.Context, artifact userWindowsArtifact, target string) ([]byte, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DisableCompression = true
	client := &http.Client{Transport: transport, Timeout: 90 * time.Second}
	defer transport.CloseIdleConnections()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 4 || req.URL.Scheme != "https" || req.URL.User != nil {
			return errors.New("Windows supplier redirect refused")
		}
		host := req.URL.Hostname()
		if host != "nodejs.org" && host != "dl.google.com" && host != "release-assets.githubusercontent.com" {
			return errors.New("Windows supplier redirect host refused")
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, artifact.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept-Encoding", "identity")
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.ContentLength > artifact.Bound {
		return nil, errors.New("Windows supplier status/byte bound refused")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, artifact.Bound+1))
	if err != nil || int64(len(data)) > artifact.Bound || (artifact.Size != 0 && int64(len(data)) != artifact.Size) || userWindowsSHA(data) != artifact.SHA {
		return nil, errors.Join(err, errors.New("Windows supplier digest/size differs"))
	}
	if err := userWindowsWrite(target, data); err != nil {
		return nil, err
	}
	return data, nil
}

// Parsing/extraction is reached only inside the qualified Windows worker.
// ZIP names, link modes, duplicates and aggregate inflation are checked before
// publishing any extracted object. Archives remain retained for later readback.
func userWindowsZIP(ctx context.Context, data []byte, artifact userWindowsArtifact, destination, member string) ([]byte, error) {
	if int64(len(data)) > artifact.Bound || userWindowsSHA(data) != artifact.SHA || ctx.Err() != nil {
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
		if ctx.Err() != nil || name == "" || (name == artifact.Prefix && !file.FileInfo().IsDir()) || seen[key] || file.Mode()&os.ModeSymlink != 0 || strings.ContainsAny(name, "\\:\x00\r\n") || (name != artifact.Prefix && !strings.HasPrefix(name, artifact.Prefix+"/")) || (!file.Mode().IsRegular() && !file.FileInfo().IsDir()) {
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
	var selected []byte
	for _, file := range reader.File {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if file.FileInfo().IsDir() || (member != "" && file.Name != member) {
			continue
		}
		stream, err := file.Open()
		if err != nil {
			return nil, err
		}
		content, readErr := io.ReadAll(io.LimitReader(stream, int64(file.UncompressedSize64)+1))
		closeErr := stream.Close()
		if err := errors.Join(readErr, closeErr); err != nil || uint64(len(content)) != file.UncompressedSize64 {
			return nil, errors.Join(err, errors.New("Windows archive physical member differs"))
		}
		if member != "" {
			selected = content
			continue
		}
		relative := strings.TrimPrefix(file.Name, artifact.Prefix+"/")
		if relative == file.Name {
			return nil, errors.New("Windows archive root is not a regular member")
		}
		target := filepath.Join(destination, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return nil, err
		}
		if err := userWindowsWrite(target, content); err != nil {
			return nil, err
		}
	}
	if member != "" && selected == nil {
		return nil, fmt.Errorf("Windows archive member missing: %s", member)
	}
	return selected, nil
}

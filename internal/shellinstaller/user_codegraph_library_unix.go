//go:build linux || darwin

package shellinstaller

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// userCodeGraphSource pins one complete npm archive. node is the exact size of
// a vendored Node member that is consumed and never written (0 when absent).
type userCodeGraphSource struct {
	name, sri      string
	size, unpacked int64
	entries        int
	node           int64
}

// userCodeGraphAcquire is the pinned network producer; tests substitute
// fixture archives that still pass every integrity and shape check.
var userCodeGraphAcquire = userAcquireCodeGraphArchives

// Qualified Linux amd64 and macOS arm64 Separate preparation retains the
// complete profile.
// The native coordinator acquires the same sources before sealed production;
// publication consumes their authenticated DATA without network or suppliers.
func userCodeGraphPreferencesSupported(profile *UserExperience) bool {
	return profile == nil || profile.GlobalRDD == "" || userQualifiedPrivateChoices(profile) || userQualifiedLegacyExperience(profile)
}

func userCodeGraphProvision(ctx context.Context, root, mode string, profile *UserExperience) error {
	if profile != nil {
		if err := profile.Validate(); err != nil {
			return err
		}
		if !userCodeGraphPreferencesSupported(profile) {
			return experienceError("explicit global RDD requires a supported native distribution")
		}
	}
	if profile == nil || profile.CodeGraph != "on" {
		return nil
	}
	if ctx == nil {
		return privateError("refused", errors.New("missing CodeGraph acquisition context"))
	}
	if err := ctx.Err(); err != nil {
		return privateError("canceled", err)
	}
	if (mode != "separate" && mode != "shared") || !userCodeGraphPlatform() || !privateHierarchyPath(root) {
		return privateError("refused", errors.New("CodeGraph library requires a live "+userCodeGraphPlatformName+" Separate or Shared stage"))
	}
	if _, err := privateDirectory(filepath.Join(root, "runtime")); err != nil {
		return err
	}
	archives, err := userCodeGraphAcquire(ctx)
	if err != nil {
		return err
	}
	return userStageCodeGraphLibrary(ctx, root, archives)
}

func userAcquireCodeGraphArchives(ctx context.Context) ([][]byte, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, privateError("canceled", context.Canceled)
	}
	client, err := privateColdClient()
	if err != nil {
		return nil, err
	}
	transport := client.Transport.(*http.Transport)
	transport.TLSClientConfig = transport.TLSClientConfig.Clone()
	transport.TLSClientConfig.ServerName = "registry.npmjs.org"
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	defer client.CloseIdleConnections()
	archives := make([][]byte, 0, len(userCodeGraphSources))
	for _, source := range userCodeGraphSources {
		endpoint := "https://registry.npmjs.org/@colbymchenry/" + source.name + "/-/" + source.name + "-1.4.1.tgz"
		origin, err := url.Parse(endpoint)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		response, err := client.Do(req)
		if err != nil {
			return nil, privateError("acquisition", err)
		}
		encoding := response.Header.Values("Content-Encoding")
		if !gentleShellStableResponseMatches(response, origin, origin.Path) || response.StatusCode != http.StatusOK || response.Uncompressed || len(encoding) > 1 || (len(encoding) == 1 && encoding[0] != "" && encoding[0] != "identity") {
			return nil, privateError("acquisition", errors.Join(response.Body.Close(), ctx.Err(), errors.New("CodeGraph source response refused")))
		}
		data, readErr := io.ReadAll(io.LimitReader(response.Body, source.size+1))
		if err := errors.Join(readErr, response.Body.Close(), ctx.Err()); err != nil {
			return nil, privateError("acquisition", err)
		}
		hash := sha512.Sum512(data)
		if int64(len(data)) != source.size || "sha512-"+base64.StdEncoding.EncodeToString(hash[:]) != source.sri {
			return nil, privateError("source", errors.New("CodeGraph archive integrity differs"))
		}
		archives = append(archives, data)
	}
	return archives, nil
}

func userCodeGraphMembers(ctx context.Context, archives [][]byte) ([]userExperienceStageFile, error) {
	if ctx == nil || len(archives) != len(userCodeGraphSources) {
		return nil, privateError("source", errors.New("CodeGraph source set differs"))
	}
	owned := make([][]byte, len(archives))
	for i, source := range userCodeGraphSources {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if int64(len(archives[i])) != source.size {
			return nil, privateError("source", errors.New("CodeGraph archive size differs"))
		}
		owned[i] = bytes.Clone(archives[i])
		hash := sha512.Sum512(owned[i])
		if "sha512-"+base64.StdEncoding.EncodeToString(hash[:]) != source.sri {
			return nil, privateError("source", errors.New("CodeGraph archive integrity differs"))
		}
	}
	var files []userExperienceStageFile
	for i, source := range userCodeGraphSources {
		gz, err := gzip.NewReader(bytes.NewReader(owned[i]))
		if err != nil {
			return nil, err
		}
		// Exact pinned payload bytes plus tar headers/padding, not a larger
		// generic tool/file bound. The oversized Node is consumed, never written.
		bounded := &io.LimitedReader{R: gz, N: source.unpacked + int64(source.entries)*1024 + 1025}
		reader := tar.NewReader(bounded)
		seen := make(map[string]bool)
		var total int64
		var metadata []byte
		for {
			header, readErr := reader.Next()
			if readErr == io.EOF {
				break
			}
			if readErr != nil || ctx.Err() != nil || header.Typeflag != tar.TypeReg || !strings.HasPrefix(header.Name, "package/") || filepath.Clean(header.Name) != header.Name || strings.ContainsAny(header.Name, "\\\x00") || seen[header.Name] || len(seen) >= source.entries {
				return nil, errors.Join(readErr, ctx.Err(), gz.Close(), errors.New("CodeGraph member refused"))
			}
			seen[header.Name] = true
			total += header.Size
			if source.node > 0 && header.Name == "package/node" && header.Size == source.node {
				continue
			}
			if header.Size < 0 || header.Size > 32<<20 {
				return nil, errors.Join(gz.Close(), errors.New("CodeGraph materialized member exceeds 32MiB"))
			}
			data, err := io.ReadAll(reader)
			if err != nil || int64(len(data)) != header.Size {
				return nil, errors.Join(err, gz.Close(), errors.New("CodeGraph member bytes differ"))
			}
			if header.Name == "package/package.json" {
				metadata = data
			}
			files = append(files, userExperienceStageFile{"node_modules/@colbymchenry/" + source.name + "/" + strings.TrimPrefix(header.Name, "package/"), data})
		}
		_, tailErr := io.Copy(io.Discard, bounded)
		if err := errors.Join(tailErr, gz.Close(), ctx.Err()); err != nil || bounded.N == 0 || total != source.unpacked || len(seen) != source.entries || (source.node > 0 && !seen["package/node"]) {
			return nil, errors.Join(err, errors.New("CodeGraph archive shape differs"))
		}
		var pkg struct{ Name, Version string }
		if json.Unmarshal(metadata, &pkg) != nil || pkg.Name != "@colbymchenry/"+source.name || pkg.Version != "1.4.1" {
			return nil, errors.New("CodeGraph package identity differs")
		}
		// Retain full source authority without creating any file above 32MiB.
		for start, part := 0, 0; start < len(owned[i]); start, part = start+(32<<20), part+1 {
			end := min(start+(32<<20), len(owned[i]))
			files = append(files, userExperienceStageFile{fmt.Sprintf("sources/%s.%d.tgz-part", source.name, part), owned[i][start:end]})
		}
	}
	return files, nil
}

func userStageCodeGraphLibrary(ctx context.Context, root string, archives [][]byte) error {
	files, err := userCodeGraphMembers(ctx, archives)
	if err != nil {
		return err
	}
	if !privateHierarchyPath(root) {
		return privateError("refused", errors.New("CodeGraph requires an absolute private stage"))
	}
	base := filepath.Join(root, "runtime")
	identity, err := privateDirectory(base)
	if err != nil {
		return err
	}
	directory, err := os.OpenRoot(base)
	if err != nil {
		return err
	}
	defer directory.Close()
	opened, err := directory.Stat(".")
	if err != nil || !os.SameFile(identity, opened) {
		return privateError("preimage", err)
	}
	dirs := map[string]bool{"codegraph": true}
	for _, file := range files {
		for name := filepath.Dir("codegraph/" + file.name); name != "."; name = filepath.Dir(name) {
			dirs[name] = true
		}
	}
	names := make([]string, 0, len(dirs))
	for name := range dirs {
		names = append(names, name)
	}
	sort.Strings(names)
	identities := map[string]os.FileInfo{base: identity}
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := directory.Mkdir(name, 0700); err != nil {
			return err
		}
		fresh, err := privateDirectory(filepath.Join(base, name))
		if err != nil {
			return err
		}
		identities[filepath.Join(base, name)] = fresh
	}
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := "codegraph/" + file.name
		if err := userWriteWithSync(name, file.data, 0600, func(path string, flags int, mode os.FileMode) (userSyncedFile, error) {
			return directory.OpenFile(path, flags, mode)
		}); err != nil {
			return err
		}
		pin := fmt.Sprintf("%x", sha256.Sum256(file.data))
		if _, err := privateNativeFile(ctx, filepath.Join(base, name), 0600, int64(len(file.data)), pin); err != nil {
			return err
		}
	}
	for path, before := range identities {
		fresh, err := privateDirectory(path)
		if err != nil || !os.SameFile(before, fresh) {
			return privateError("preimage", err)
		}
	}
	return nil
}

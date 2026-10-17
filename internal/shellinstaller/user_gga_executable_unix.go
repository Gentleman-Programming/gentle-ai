//go:build linux || darwin

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const userGGASourceSHA = "fb681f48706be5ee8f7d1efd558fdfacbc7757aaf4ca9d8fd05fce4c44d7d004"
const userGGASourcePrefix = "gentleman-guardian-angel-1124c3672f082c56b033c4e23a30e95d0e8cd593/"

// Fixed acquisition into an unpublished stage only. Public profile admission
// stays refused; managed launch/readback and provider setup are separate gates.
func userGGAProvision(ctx context.Context, root, mode string, profile *UserExperience) error {
	if err := userValidateStagePreferences(profile); err != nil {
		return err
	}
	if profile == nil || profile.GGA != "on" {
		return nil
	}
	if mode != "separate" || ctx == nil {
		return experienceError("GGA provisioning requires a live private Separate stage")
	}
	if err := ctx.Err(); err != nil {
		return privateError("canceled", err)
	}
	client, err := privateColdClient()
	if err != nil {
		return err
	}
	transport := client.Transport.(*http.Transport)
	transport.TLSClientConfig = transport.TLSClientConfig.Clone()
	transport.TLSClientConfig.ServerName = "codeload.github.com"
	defer client.CloseIdleConnections()
	return userGGAAcquire(ctx, root, client)
}

// The client seam permits offline response controls, not an alternate endpoint
// or trust store. Production supplies the cold system-rooted TLS transport.
func userGGAAcquire(ctx context.Context, root string, client *http.Client) error {
	if ctx == nil || client == nil {
		return privateError("acquisition", errors.New("missing GGA acquisition context/client"))
	}
	if err := ctx.Err(); err != nil {
		return privateError("acquisition", err)
	}
	const endpoint = "https://codeload.github.com/Gentleman-Programming/gentleman-guardian-angel/tar.gz/1124c3672f082c56b033c4e23a30e95d0e8cd593"
	origin, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	bounded := *client
	bounded.Jar = nil
	bounded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := bounded.Do(req)
	if err != nil {
		return privateError("acquisition", err)
	}
	encoding := response.Header.Values("Content-Encoding")
	if !gentleShellStableResponseMatches(response, origin, origin.Path) || response.StatusCode != http.StatusOK || response.Uncompressed || len(encoding) > 1 || (len(encoding) == 1 && encoding[0] != "" && encoding[0] != "identity") {
		return privateError("acquisition", errors.Join(response.Body.Close(), ctx.Err(), errors.New("GGA source response refused")))
	}
	// Codeload can omit Content-Length; exact bounded bytes and the independent
	// frozen archive SHA remain mandatory. No redirects, proxy or decompression.
	archive, readErr := io.ReadAll(io.LimitReader(response.Body, 81842))
	if err := errors.Join(readErr, response.Body.Close(), ctx.Err()); err != nil {
		return privateError("acquisition", err)
	}
	if len(archive) != 81841 {
		return privateError("acquisition", errors.New("GGA archive size differs"))
	}
	return userStageGGAExecutable(ctx, root, archive)
}

func userStageGGAExecutable(ctx context.Context, root string, archive []byte) error {
	if ctx == nil {
		return privateError("refused", errors.New("GGA executable requires a live absolute private stage"))
	}
	if err := ctx.Err(); err != nil {
		return privateError("canceled", err)
	}
	if !privateHierarchyPath(root) {
		return privateError("refused", errors.New("GGA executable requires a live absolute private stage"))
	}
	files, err := userGGAMembers(ctx, archive)
	if err != nil {
		return err
	}
	runtime := filepath.Join(root, "runtime")
	identity, err := privateDirectory(runtime)
	if err != nil {
		return err
	}
	directory, err := os.OpenRoot(runtime)
	if err != nil {
		return err
	}
	defer directory.Close()
	opened, err := directory.Stat(".")
	if err != nil || !os.SameFile(identity, opened) {
		return privateError("preimage", err)
	}
	identities := map[string]os.FileInfo{runtime: identity}
	for _, name := range []string{"gga", "gga/bin", "gga/lib"} {
		if err := directory.Mkdir(name, 0700); err != nil {
			return err
		}
		path := filepath.Join(runtime, name)
		created, err := privateDirectory(path)
		if err != nil {
			return err
		}
		identities[path] = created
	}
	files = append(files, userExperienceStageFile{"source.tgz", archive})
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		mode := os.FileMode(0700)
		if file.name == "source.tgz" {
			mode = 0600
		}
		name := "gga/" + file.name
		if err := userWriteWithSync(name, file.data, mode, func(path string, flags int, mode os.FileMode) (userSyncedFile, error) {
			return directory.OpenFile(path, flags, mode)
		}); err != nil {
			return err
		}
		pin := fmt.Sprintf("%x", sha256.Sum256(file.data))
		if _, err := privateNativeFile(ctx, filepath.Join(runtime, name), mode, int64(len(file.data)), pin); err != nil {
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

// One authenticated recipe drives initial publication and retained readback.
func userGGAMembers(ctx context.Context, archive []byte) ([]userExperienceStageFile, error) {
	files := make([]userExperienceStageFile, 0, 4)
	for _, leaf := range []string{"bin/gga", "lib/providers.sh", "lib/cache.sh", "lib/pr_mode.sh"} {
		data, err := userToolMember(ctx, archive, userGGASourceSHA, userGGASourcePrefix+leaf)
		if err != nil {
			return nil, err
		}
		if leaf == "bin/gga" {
			const original = "VERSION=\"${GGA_VERSION:-dev}\""
			if strings.Count(string(data), original) != 1 {
				return nil, privateError("source", errors.New("GGA version recipe preimage differs"))
			}
			// Match upstream's tag-stamping installation recipe, not an environment
			// claim. The original authenticated source archive is retained below.
			data = []byte(strings.Replace(string(data), original, "VERSION=\"2.10.1\"", 1))
		}
		files = append(files, userExperienceStageFile{leaf, data})
	}
	return files, nil
}

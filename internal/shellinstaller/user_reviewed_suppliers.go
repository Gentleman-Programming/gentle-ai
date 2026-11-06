package shellinstaller

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	assets "github.com/gentleman-programming/gentle-ai/v4/scripts"
)

// The reviewed gentle-pi supplier list: one JSON file in the gentle-ai
// repository, embedded in every installer build and read live from the
// repository main branch at check and update time. A release outside the list
// never breaks an install or an update; the newest listed release is the
// stable target, and any fetch or validation failure silently falls back to
// the embedded copy — it is never an error for the user.

const (
	userReviewedListSchema = "gentle-shell-reviewed-suppliers/v1"
	userReviewedListURL    = "https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/main/scripts/gentle-pi-suppliers.json"
	userReviewedListBound  = 256 << 10
	userReviewedListAgent  = "gentle-shell-installer"
)

type userReviewedSupplier struct {
	Integrity string `json:"integrity"`
	Installer string `json:"installer"`
	Binary    string `json:"binary"`
}

// userReviewedList is the strictly validated form of the reviewed supplier
// list: an exact schema string, at least one release keyed by exact semver,
// every entry binding the npm tarball SRI and the SHA-256 of the two native
// supplier files.
type userReviewedList struct {
	Schema   string                          `json:"schema"`
	Releases map[string]userReviewedSupplier `json:"releases"`
}

var userReviewedListSemverPattern = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$`)

func userReviewedListSemver(version string) bool {
	return userReviewedListSemverPattern.MatchString(version)
}

// userReviewedListSRI admits only a well-formed sha512 SRI whose base64
// round-trips exactly, mirroring the provisioners' lock integrity gate.
func userReviewedListSRI(value string) bool {
	if len(value) != 95 || !strings.HasPrefix(value, "sha512-") {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(value[7:])
	return err == nil && len(raw) == 64 && base64.StdEncoding.EncodeToString(raw) == value[7:]
}

func userParseReviewedList(data []byte) (userReviewedList, error) {
	var decoded userReviewedList
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return userReviewedList{}, fmt.Errorf("reviewed supplier list: %w", err)
	}
	if decoder.More() {
		return userReviewedList{}, errors.New("reviewed supplier list: trailing content refused")
	}
	if decoded.Schema != userReviewedListSchema || len(decoded.Releases) == 0 {
		return userReviewedList{}, errors.New("reviewed supplier list: schema or release set refused")
	}
	list := userReviewedList{Schema: decoded.Schema, Releases: make(map[string]userReviewedSupplier, len(decoded.Releases))}
	for version, entry := range decoded.Releases {
		if !userReviewedListSemver(version) {
			return userReviewedList{}, fmt.Errorf("reviewed supplier list: release key %q is not an exact semver", version)
		}
		if !userReviewedListSRI(entry.Integrity) || !userSnapshotLowerHex(entry.Installer, 64) || !userSnapshotLowerHex(entry.Binary, 64) {
			return userReviewedList{}, fmt.Errorf("reviewed supplier list: release %q binds malformed digests", version)
		}
		list.Releases[version] = entry
	}
	return list, nil
}

// newestReviewedRelease returns the newest listed version in semver order:
// the stable update target. Empty only for a zero list.
func (list userReviewedList) newestReviewedRelease() string {
	newest := ""
	for version := range list.Releases {
		if newest == "" || userReviewedListVersionLess(newest, version) {
			newest = version
		}
	}
	return newest
}

func userReviewedListVersionLess(left, right string) bool {
	parse := func(version string) (a, b, c int) {
		parts := strings.SplitN(version, ".", 3)
		a, _ = strconv.Atoi(parts[0])
		b, _ = strconv.Atoi(parts[1])
		c, _ = strconv.Atoi(parts[2])
		return a, b, c
	}
	l0, l1, l2 := parse(left)
	r0, r1, r2 := parse(right)
	if l0 != r0 {
		return l0 < r0
	}
	if l1 != r1 {
		return l1 < r1
	}
	return l2 < r2
}

// userReviewedVersionOlder reports whether an installed gentle-pi version is
// strictly older than the update target, or unrecorded: only then does
// detection report the Stable sdk outdated. A newer installed version, e.g.
// after a fallback to an older embedded list, never offers a downgrade. The
// installed value comes from installation.json without validation, so a
// malformed one counts as unrecorded instead of reaching the comparator.
func userReviewedVersionOlder(installed, target string) bool {
	if installed == "" || !userReviewedListSemver(installed) {
		return true
	}
	return userReviewedListVersionLess(installed, target)
}

// userEmbeddedReviewedList parses the embedded copy: the offline fallback and
// the authority for the frozen lock's supplier pins. A refusal here is a
// build defect, so it is reported, never silently absorbed.
func userEmbeddedReviewedList() (userReviewedList, []byte, error) {
	data, err := assets.ReadReviewedSuppliers()
	if err != nil {
		return userReviewedList{}, nil, err
	}
	list, err := userParseReviewedList(data)
	if err != nil {
		return userReviewedList{}, nil, err
	}
	return list, data, nil
}

// One fixed-publisher read; no redirects, proxy, credentials or retries,
// modeled on the bounded Main snapshot client. The client never outlives the
// caller's remaining deadline and never exceeds the 10 s budget, well inside
// the TUI check's 20 s window.
func userReviewedListClient(ctx context.Context) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy, transport.DisableCompression = nil, true
	timeout := 10 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining < timeout {
			timeout = remaining
		}
	}
	return &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("reviewed supplier list publisher redirect refused")
	}}
}

// userReviewedListUsing performs the bounded GET of the fixed URL. The client
// seam keeps the same fixed URL; offline response DATA do not prove TLS.
func userReviewedListUsing(ctx context.Context, client *http.Client) (data []byte, resultErr error) {
	if ctx == nil {
		return nil, context.Canceled
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if client == nil {
		return nil, errors.New("reviewed supplier list acquisition requires a live context and client")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userReviewedListURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", userReviewedListAgent)
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, response.Body.Close()) }()
	switch {
	case response.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("reviewed supplier list publisher %s returned HTTP %d", req.URL.Host, response.StatusCode)
	case response.ContentLength > userReviewedListBound:
		return nil, fmt.Errorf("reviewed supplier list publisher %s response exceeds %d bytes", req.URL.Host, userReviewedListBound)
	case response.Header.Get("Content-Encoding") != "" && response.Header.Get("Content-Encoding") != "identity":
		return nil, fmt.Errorf("reviewed supplier list publisher %s used Content-Encoding %q", req.URL.Host, response.Header.Get("Content-Encoding"))
	}
	data, err = io.ReadAll(io.LimitReader(response.Body, userReviewedListBound+1))
	if err != nil || int64(len(data)) > userReviewedListBound {
		return nil, errors.Join(err, errors.New("reviewed supplier list response bound"))
	}
	return data, nil
}

// userSelectedReviewedList resolves the list authority for one check or
// install: the live repository copy when fetch and strict validation succeed,
// otherwise the embedded copy. Fetch or validation failure is never an error
// for the user. Only an unreadable embedded copy (a build defect) fails.
func userSelectedReviewedList(ctx context.Context) (userReviewedList, []byte, error) {
	client := userReviewedListClient(ctx)
	defer client.CloseIdleConnections()
	return userSelectedReviewedListUsing(ctx, client)
}

func userSelectedReviewedListUsing(ctx context.Context, client *http.Client) (userReviewedList, []byte, error) {
	embeddedList, embeddedBytes, err := userEmbeddedReviewedList()
	if err != nil {
		return userReviewedList{}, nil, err
	}
	live, liveErr := userReviewedListUsing(ctx, client)
	if liveErr == nil {
		if list, parseErr := userParseReviewedList(live); parseErr == nil {
			return list, live, nil
		}
	}
	// Only a canceled check aborts like every other canceled acquisition; a
	// caller deadline and every other fetch or validation failure use the
	// embedded copy silently, so the check never fails for the user.
	if errors.Is(liveErr, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return userReviewedList{}, nil, context.Canceled
	}
	return embeddedList, embeddedBytes, nil
}

// userSelectedReviewedListFunc is the injection seam for the selected list at
// the call sites (Windows stage write and stable update check). Production
// resolves the real bounded client; tests stub it, so no test touches the
// network.
var userSelectedReviewedListFunc = userSelectedReviewedList

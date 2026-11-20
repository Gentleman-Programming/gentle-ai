package shellinstaller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// userMainFallbackCommit is the last Main snapshot verified against the stock
// composition. HEAD is attempted first; this commit is used only when the TUI
// retries after a Main composition failure.
const userMainFallbackCommit = "63d6065f924ec366a321dd21f7396f04026c026f"

// ErrMainSDKInstall marks a failed Main SDK composition. Callers match it
// with errors.Is to offer the last qualified Main snapshot; the wording of
// the wrapped error is never part of that contract.
var ErrMainSDKInstall = errors.New("Main SDK installation failed")

// userMainSDKError keeps the existing "<context>: <cause>" message while
// matching ErrMainSDKInstall and unwrapping to the cause.
type userMainSDKError struct {
	context string
	err     error
}

func (e *userMainSDKError) Error() string {
	if e.context == "" {
		return e.err.Error()
	}
	return e.context + ": " + e.err.Error()
}
func (e *userMainSDKError) Unwrap() error        { return e.err }
func (e *userMainSDKError) Is(target error) bool { return target == ErrMainSDKInstall }

// MainSDKFailureMarker is the stderr line a child installer prints when it
// fails with ErrMainSDKInstall. Linux and Windows install in a child process,
// so the typed error crosses the process boundary only as this line.
const MainSDKFailureMarker = "gentle-ai shell: Main SDK composition failed"

// MainSDKFailureFromWorker restores ErrMainSDKInstall on a child worker's
// error when the worker output carries MainSDKFailureMarker on its own line.
// The error message is unchanged; any other error is returned as is.
func MainSDKFailureFromWorker(err error, output string) error {
	if err == nil || errors.Is(err, ErrMainSDKInstall) {
		return err
	}
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == MainSDKFailureMarker {
			return &userMainSDKError{err: err}
		}
	}
	return err
}

// userMainSDKFailure wraps a Main SDK composition failure; nil stays nil.
func userMainSDKFailure(context string, err error) error {
	if err == nil {
		return nil
	}
	return &userMainSDKError{context: context, err: err}
}

type userSnapshotArtifact struct {
	URL, SHA, Archive, Prefix string
	Bound, Size               int64
}

// Descriptor construction alone does not authenticate or resolve Main source.
func userMainDescriptor(commit, archiveSHA string) (userSnapshotArtifact, error) {
	if !userSnapshotLowerHex(commit, 40) || !userSnapshotLowerHex(archiveSHA, 64) {
		return userSnapshotArtifact{}, errors.New("invalid frozen Gentle Shell Main commit/archive digest")
	}
	return userSnapshotArtifact{URL: "https://codeload.github.com/Gentleman-Programming/gentle-shell/zip/" + commit, SHA: archiveSHA, Archive: "main.zip", Prefix: "gentle-shell-" + commit, Bound: 32 << 20}, nil
}

// userMainSnapshotFunc is the update check's Main snapshot authority; tests
// stub it so the detection never touches the network.
var userMainSnapshotFunc = userMainSnapshot

func userSnapshotLowerHex(value string, length int) bool {
	_, err := hex.DecodeString(value)
	return len(value) == length && err == nil && value == strings.ToLower(value)
}

// One fixed-publisher ref lookup; no redirects, proxy, credentials or retries.
func userMainSnapshot(ctx context.Context) (userSnapshotArtifact, []byte, error) {
	if ctx == nil {
		return userSnapshotArtifact{}, nil, context.Canceled
	}
	if err := ctx.Err(); err != nil {
		return userSnapshotArtifact{}, nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy, transport.DisableCompression = nil, true
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("Main publisher redirect refused")
	}}
	return userMainSnapshotUsing(ctx, client)
}

// userMainPublisherRefusal names what a publisher response failed on. GitHub
// limits unauthenticated API calls per network, so 403/429 are reported as a
// rate limit instead of an opaque refusal.
func userMainPublisherRefusal(host string, response *http.Response, bound int64) error {
	switch {
	case response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusTooManyRequests:
		return fmt.Errorf("Main publisher %s returned HTTP %d: GitHub rate limit for unauthenticated requests from this network; retry later or install Stable", host, response.StatusCode)
	case response.StatusCode != http.StatusOK:
		return fmt.Errorf("Main publisher %s returned HTTP %d", host, response.StatusCode)
	case response.ContentLength > bound:
		return fmt.Errorf("Main publisher %s response exceeds %d bytes", host, bound)
	}
	if encoding := response.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		return fmt.Errorf("Main publisher %s used Content-Encoding %q", host, encoding)
	}
	return nil
}

// Client seam keeps the same fixed URLs; offline response DATA do not prove TLS.
func userMainSnapshotUsing(ctx context.Context, client *http.Client) (userSnapshotArtifact, []byte, error) {
	if ctx == nil {
		return userSnapshotArtifact{}, nil, context.Canceled
	}
	if err := ctx.Err(); err != nil {
		return userSnapshotArtifact{}, nil, err
	}
	if client == nil {
		return userSnapshotArtifact{}, nil, errors.New("Main acquisition requires a live context and client")
	}
	get := func(url, accept string, bound int64) (content []byte, resultErr error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		req.Header.Set("Accept-Encoding", "identity")
		req.Header.Set("User-Agent", "gentle-shell-windows-installer")
		response, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer func() { resultErr = errors.Join(resultErr, response.Body.Close()) }()
		if err := userMainPublisherRefusal(req.URL.Host, response, bound); err != nil {
			return nil, err
		}
		// GitHub streams without a Content-Length, so the oversize check above
		// cannot fire; an oversize body and a failed read are named apart.
		data, err := io.ReadAll(io.LimitReader(response.Body, bound+1))
		if err != nil {
			return nil, fmt.Errorf("Main publisher %s response read failed: %w", req.URL.Host, err)
		}
		if int64(len(data)) > bound {
			return nil, fmt.Errorf("Main publisher %s response exceeds %d bytes", req.URL.Host, bound)
		}
		return data, nil
	}
	commit := ""
	if os.Getenv("GENTLE_AI_MAIN_FALLBACK") == "1" {
		commit = userMainFallbackCommit
	} else {
		// The bare-SHA media type answers with the 40-character SHA alone; the
		// default commit JSON embeds the newest commit's files and patches, so
		// its size followed whatever that commit touched (#5987).
		ref, err := get("https://api.github.com/repos/Gentleman-Programming/gentle-shell/commits/main", "application/vnd.github.sha", 64)
		if err != nil {
			return userSnapshotArtifact{}, nil, err
		}
		if !userSnapshotLowerHex(string(ref), 40) {
			return userSnapshotArtifact{}, nil, errors.New("canonical Main ref identity refused")
		}
		commit = string(ref)
	}
	data, err := get("https://codeload.github.com/Gentleman-Programming/gentle-shell/zip/"+commit, "", 32<<20)
	if err != nil {
		return userSnapshotArtifact{}, nil, err
	}
	// Body.Close has settled before get returns; cancellation there must not
	// publish a usable snapshot to the confirmed installer.
	if err := ctx.Err(); err != nil {
		return userSnapshotArtifact{}, nil, err
	}
	artifact, err := userMainDescriptor(commit, fmt.Sprintf("%x", sha256.Sum256(data)))
	return artifact, data, err
}

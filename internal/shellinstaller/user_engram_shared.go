//go:build linux || darwin || windows

package shellinstaller

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Shared Engram memory for a Separate launch, ported from Gentle Shell's
// bundled policy (lib/gentle-shell-bundled.ts classifyEngramStartup and
// bin/gentle-shell.mjs startSharedEngram). Like other agents, a Separate
// install shares the user's global Engram server: a verified listener on the
// default endpoint is reused, and with nothing listening the installation's
// verified core starts one on Engram's defaults with the user's real HOME.
// Only the outer entry process decides, before the contained worker starts:
// a server started inside the Job Object or systemd unit would die with it.
// Memory never fails a launch; any doubt disables it for that launch.

// userEngramDefaultURL is the endpoint gentle-engram uses without ENGRAM_URL
// or ENGRAM_PORT, and the only URL a decision ever forwards.
const userEngramDefaultURL = "http://127.0.0.1:7437"

// userEngramProbeBase is where the decision probes; tests point it at a fake
// server so they never contact the real endpoint.
var userEngramProbeBase = userEngramDefaultURL

// userEngramSelections are the user's explicit Engram choices (Shell's
// ENGRAM_SELECTIONS): any one of them is forwarded as given, never decided.
var userEngramSelections = []string{"ENGRAM_BIN", "ENGRAM_URL", "ENGRAM_DATA_DIR", "ENGRAM_PORT", "ENGRAM_SOCKET", "ENGRAM_DATABASE_URL", "ENGRAM_SERVER_MODE"}

const (
	userEngramPolicyEnv = "ENGRAM_STARTUP_POLICY"
	userEngramNoticeEnv = "ENGRAM_STARTUP_NOTICE"
)

// From Engram's store schemaVersion; a newer store refuses a start, never enables an unsafe one.
const userEngramCoreSchema = 1

const (
	userEngramRecoveryNotice    = "Engram memory disabled: Gentle Shell could not verify safe reuse of the server on 127.0.0.1:7437 (no verified /compatibility evidence) and never stops or replaces a server already listening there. Back up your Engram data before manual changes. Gentle Shell rechecks on every launch and remains usable without memory."
	userEngramCoreMissingNotice = "Engram memory disabled: this Separate installation has no verified Engram core yet. Reinstall it with gentle-ai shell install to provision the core, then relaunch. Gentle Shell remains usable without memory."
	userEngramNewerSchemaNotice = "Engram memory disabled: ~/.engram was migrated by a newer Engram than this installation's core, so Gentle Shell will not start a server on it. Start that newer Engram's server yourself (engram serve); a running server with verified /compatibility evidence is reused. Back up the store; do not delete or downgrade it. Gentle Shell remains usable without memory."
	userEngramNoDecisionNotice  = "Engram memory disabled: this launch path did not receive Gentle Shell's verified shared-memory decision, so it will not run an unverified Engram from PATH. Gentle Shell remains usable without memory."
	userEngramStartFailedNotice = "Engram memory disabled for this launch: Gentle Shell could not start the shared Engram server, or it did not report ready in time. Relaunch to retry; if another program holds 127.0.0.1:7437, stop it first. Gentle Shell remains usable without memory."
)

// userEngramProbeTimeout bounds one /compatibility request of a decision.
const userEngramProbeTimeout = time.Second

// userEngramAllow is the launch environment of a verified shared server.
var userEngramAllow = []string{userEngramPolicyEnv + "=allow", "ENGRAM_URL=" + userEngramDefaultURL}

type userEngramEndpoint int

const (
	userEngramUnknown userEngramEndpoint = iota
	userEngramRefused
	userEngramIncompatible
	userEngramCompatible
)

// userEngramHost is everything a decision may observe or cause; production
// hosts are per platform, tests inject fakes.
type userEngramHost struct {
	lookup func(string) (string, bool)
	probe  func(context.Context, time.Duration) userEngramEndpoint
	// core returns the revalidated core, or false when it is absent or doubtful.
	core func() (string, bool)
	// header returns the real ~/.engram/engram.db user_version when readable.
	header func() (uint32, bool)
	// start launches `core serve` detached; exited closes when it ends.
	start func(core string) (exited <-chan struct{}, err error)
	// Readiness bounds; zero selects Shell's 10s, 2s and 100ms.
	timeout, grace, interval time.Duration
}

func userEngramDisabled(notice string) []string {
	return []string{userEngramPolicyEnv + "=disabled", userEngramNoticeEnv + "=" + notice}
}

// userEngramDecide returns the Engram entries for the launch environment.
func userEngramDecide(ctx context.Context, host userEngramHost) []string {
	policy, policySet := host.lookup(userEngramPolicyEnv)
	selected := policySet && policy != "allow"
	for _, key := range userEngramSelections {
		if value, ok := host.lookup(key); ok && strings.TrimSpace(value) != "" {
			selected = true
		}
	}
	if selected {
		return userEngramSelected(host)
	}
	switch host.probe(ctx, userEngramProbeTimeout) {
	case userEngramCompatible:
		return userEngramAllow
	case userEngramRefused:
	default:
		return userEngramDisabled(userEngramRecoveryNotice)
	}
	core, ok := host.core()
	if !ok {
		return userEngramDisabled(userEngramCoreMissingNotice)
	}
	if version, ok := host.header(); ok && version > userEngramCoreSchema {
		return userEngramDisabled(userEngramNewerSchemaNotice)
	}
	timeout, grace, interval := host.timeout, host.grace, host.interval
	if timeout == 0 {
		timeout, grace, interval = 10*time.Second, 2*time.Second, 100*time.Millisecond
	}
	exited, err := host.start(core)
	if err != nil {
		return userEngramDisabled(userEngramStartFailedNotice)
	}
	// A concurrent launch that wins the bind converges too: after the child
	// exits, polling continues for a short grace within the overall bound.
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		if host.probe(ctx, 500*time.Millisecond) == userEngramCompatible {
			return userEngramAllow
		}
		select {
		case <-exited:
			exited = nil
			if shortened := time.Now().Add(grace); shortened.Before(deadline) {
				deadline = shortened
			}
		case <-time.After(interval):
		}
	}
	return userEngramDisabled(userEngramStartFailedNotice)
}

// userEngramSeparateLaunch fails closed for a Separate launch whose worker
// received no decision (no selection, URL or policy), as on the Linux
// root-risk unit: the declared companion would otherwise resolve an
// unverified engram on PATH. Other modes keep their entries unchanged.
func userEngramSeparateLaunch(mode string, entries []string) []string {
	if mode != "separate" {
		return entries
	}
	for _, entry := range entries {
		key, _, _ := strings.Cut(entry, "=")
		if key == userEngramPolicyEnv || slices.Contains(userEngramSelections, key) {
			return entries
		}
	}
	return userEngramDisabled(userEngramNoDecisionNotice)
}

// userEngramSelected ports bundled Shell's rule for an explicit selection:
// it is forwarded untouched and memory is disabled (a user's own non-allow
// policy is kept as given). An explicit ENGRAM_BIN is never replaced; without
// one, ENGRAM_BIN names the verified core when it revalidates (bundled names
// its core or its guard, which Separate does not have), and otherwise the
// disabled policy keeps the companion from running any engram, so PATH is
// never the source.
func userEngramSelected(host userEngramHost) []string {
	var env []string
	for _, key := range userEngramSelections {
		if value, ok := host.lookup(key); ok && value != "" {
			env = append(env, key+"="+value)
		}
	}
	if bin, ok := host.lookup("ENGRAM_BIN"); !ok || strings.TrimSpace(bin) == "" {
		if core, ok := host.core(); ok {
			env = append(env, "ENGRAM_BIN="+core)
		}
	}
	notice, _ := host.lookup(userEngramNoticeEnv)
	if policy, ok := host.lookup(userEngramPolicyEnv); ok && policy != "allow" {
		env = append(env, userEngramPolicyEnv+"="+policy)
		if notice != "" {
			env = append(env, userEngramNoticeEnv+"="+notice)
		}
		return env
	}
	return append(env, userEngramDisabled(strings.TrimSpace(userEngramRecoveryNotice+" "+notice))...)
}

// userEngramForwarded is the user's own Engram configuration, as given.
func userEngramForwarded(lookup func(string) (string, bool)) []string {
	var env []string
	for _, key := range append(append([]string{}, userEngramSelections...), userEngramPolicyEnv, userEngramNoticeEnv) {
		if value, ok := lookup(key); ok && value != "" {
			env = append(env, key+"="+value)
		}
	}
	return env
}

var userEngramHex32 = regexp.MustCompile(`^[a-f0-9]{32}$`)

// userEngramReady is Shell's isReadyCompatibility: the exact contract a
// startup-verified store reports on GET /compatibility.
func userEngramReady(raw []byte) bool {
	// Exact keys, as Shell reads them: a map never matches case-insensitively.
	var body map[string]any
	if json.Unmarshal(raw, &body) != nil {
		return false
	}
	text := func(key string) string { value, _ := body[key].(string); return value }
	return text("status") == "ready" && body["active_reuse_authorized"] == true && text("capability") == "startup-verified-store-v1" &&
		body["revision"] == float64(1) && text("source") == "active" && body["offline_automatic_opening_authorized"] != true &&
		userEngramHex32.MatchString(text("instance_id")) && userEngramHex32.MatchString(text("runtime_id"))
}

// userEngramProbe is one bounded GET of base/compatibility on a direct
// socket: no proxy, redirects, credentials or keep-alive.
func userEngramProbe(ctx context.Context, base string, timeout time.Duration) userEngramEndpoint {
	client := &http.Client{
		Transport:     &http.Transport{Proxy: nil, DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Timeout:       timeout,
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/compatibility", nil)
	if err != nil {
		return userEngramUnknown
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		if errors.Is(err, userEngramRefusedErrno) {
			return userEngramRefused
		}
		return userEngramUnknown
	}
	defer response.Body.Close()
	const limit = 16 << 10
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return userEngramUnknown
	}
	if response.StatusCode != http.StatusOK || len(body) > limit || !userEngramReady(body) {
		return userEngramIncompatible
	}
	return userEngramCompatible
}

// userEngramHeaderVersion is SQLite's user_version from a raw database
// header (bytes 60-63), or false for anything that is not a well-formed
// rollback or WAL database (Shell's sqliteHeaderUserVersion).
func userEngramHeaderVersion(header []byte, size int64) (uint32, bool) {
	if len(header) < 100 || string(header[:16]) != "SQLite format 3\x00" {
		return 0, false
	}
	page := int64(binary.BigEndian.Uint16(header[16:18]))
	if page == 1 {
		page = 65536
	}
	if page < 512 || page > 65536 || page&(page-1) != 0 || size < page || size%page != 0 {
		return 0, false
	}
	if (header[18] != 1 && header[18] != 2) || (header[19] != 1 && header[19] != 2) {
		return 0, false
	}
	return binary.BigEndian.Uint32(header[60:64]), true
}

// userEngramReadHeader reads the store header as a plain file, never through
// SQLite: a regular, unlinked engram.db whose opened identity is the one seen.
func userEngramReadHeader(path string) (uint32, bool) {
	seen, err := os.Lstat(path)
	if err != nil || !seen.Mode().IsRegular() {
		return 0, false
	}
	file, err := os.OpenFile(path, os.O_RDONLY|userEngramNoFollow, 0)
	if err != nil {
		return 0, false
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(seen, opened) {
		return 0, false
	}
	header := make([]byte, 100)
	if _, err := io.ReadFull(file, header); err != nil {
		return 0, false
	}
	return userEngramHeaderVersion(header, opened.Size())
}

// userEngramQuiet is a version or help run's launch environment: memory
// disabled without a notice, as bundled Shell forwards for those runs.
var userEngramQuiet = []string{userEngramPolicyEnv + "=disabled"}

// Pi 1.1.0 options that always take the next token as their value (Shell's
// PI_VALUE_OPTIONS); options that skip a dash-led next token never hide a
// version or help flag.
var userEngramPiValueOptions = []string{"--provider", "--model", "--api-key", "--system-prompt", "--append-system-prompt", "--name", "-n", "--session",
	"--session-id", "--fork", "--session-dir", "--models", "--tools", "-t", "--exclude-tools", "-xt", "--thinking", "--export", "--extension", "-e",
	"--skill", "--prompt-template", "--theme"}

// userEngramVersionOrHelp is Shell's piVersionOrHelpRequested: a version or
// help token in flag position, before Pi's own "--" and not as another
// option's value. Shell's own --version is the same token.
func userEngramVersionOrHelp(argv []string) bool {
	for i := 0; i < len(argv); i++ {
		switch arg := argv[i]; {
		case arg == "--":
			return false
		case arg == "--version" || arg == "-v" || arg == "--help" || arg == "-h":
			return true
		case slices.Contains(userEngramPiValueOptions, arg):
			i++
		}
	}
	return false
}

// userEngramLaunchDecision is the entry decision for a launch of root with
// args: like bundled Shell, a version or help run never probes Engram, starts
// a server or prints a notice.
func userEngramLaunchDecision(ctx context.Context, root string, args []string) []string {
	if userEngramVersionOrHelp(args) {
		return userEngramQuiet
	}
	return userEngramLaunchEnv(ctx, root)
}

type userEngramLaunchKey struct{}

// userEngramWithLaunch carries the launch's Engram entries, computed by
// decide, to whatever builds the launch or worker environment.
func userEngramWithLaunch(ctx context.Context, decide func() []string) context.Context {
	return context.WithValue(ctx, userEngramLaunchKey{}, decide)
}

// userEngramContextEnv is nil for every context without a launch decision,
// so install, overlay and verify environments never change.
func userEngramContextEnv(ctx context.Context) []string {
	if decide, ok := ctx.Value(userEngramLaunchKey{}).(func() []string); ok {
		return decide()
	}
	return nil
}

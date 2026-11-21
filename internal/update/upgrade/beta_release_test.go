package upgrade

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v4/internal/update"
)

const (
	betaReleaseVersion = "4.6.0"
	betaReleaseCommit  = "0123456789abcdef0123456789abcdef01234567"
	betaNextCommit     = "89abcdef0123456789abcdef0123456789abcdef"
	betaReleaseModule  = "github.com/gentleman-programming/gentle-ai/v4"
)

func TestBetaReleaseUpgradeFollowsMainAfterTag(t *testing.T) {
	remote := fakeBetaReleaseRemote(t)
	binary, calls := fakeBetaReleaseInstall(t)
	ctx := context.Background()
	profile := linuxProfile()
	profile.GoAvailable = true
	home := t.TempDir()

	for i := 0; i < 2; i++ {
		results := update.CheckFiltered(ctx, betaReleaseVersion, profile, []string{"gentle-ai"})
		if len(results) != 1 || results[0].Status != update.UpToDate || results[0].LatestVersion != "main@0123456789ab" {
			t.Errorf("run %d: check = %+v, want up to date at tagged main", i+1, results)
		}
		report := ExecuteWithOptions(ctx, results, profile, home, false, ExecuteOptions{SkipBackup: true})
		if len(report.Results) != 0 || len(*calls) != 0 {
			t.Errorf("run %d: report = %+v, commands = %v; want no upgrade", i+1, report, *calls)
		}
		assertBetaReleaseBinary(t, binary, betaReleaseVersion)
	}

	remote.main = betaNextCommit
	results := update.CheckFiltered(ctx, betaReleaseVersion, profile, []string{"gentle-ai"})
	wantCompare := "https://github.com/Gentleman-Programming/gentle-ai/compare/0123456789ab...89abcdef0123"
	if len(results) != 1 || results[0].Status != update.UpdateAvailable || results[0].ReleaseURL != wantCompare {
		t.Errorf("after main advances: check = %+v, want update at %s", results, wantCompare)
	}
	report := ExecuteWithOptions(ctx, results, profile, home, false, ExecuteOptions{SkipBackup: true})
	if len(report.Results) != 1 || report.Results[0].Status != UpgradeSucceeded || report.Results[0].NewVersion != "main@89abcdef0123" {
		t.Errorf("after main advances: report = %+v, want successful upgrade", report)
	}
	wantCalls := []string{"go env GOBIN", "go install " + betaReleaseModule + "/cmd/gentle-ai@" + betaNextCommit}
	if !slices.Equal(*calls, wantCalls) {
		t.Errorf("commands = %v, want %v", *calls, wantCalls)
	}
	assertBetaReleaseBinary(t, binary, betaNextCommit)
	if remote.tagRequests != 3 || remote.compareRequests != 1 {
		t.Errorf("tag requests = %d, compare requests = %d; want three tag resolutions and one comparison", remote.tagRequests, remote.compareRequests)
	}
}

func TestBetaReleaseDoesNotUpgradeWithoutProvenAdvancement(t *testing.T) {
	for _, tc := range []struct {
		name, compareBody, wantHint string
		compareStatus               int
	}{
		{
			name:        "main rolled back to the release parent",
			compareBody: `{"status":"behind","ahead_by":0,"behind_by":1}`,
			wantHint:    "Beta main is behind the installed release; keep the installed binary.",
		},
		{
			name:        "main diverged",
			compareBody: `{"status":"diverged","ahead_by":2,"behind_by":1}`,
			wantHint:    "Beta main has diverged from the installed release; keep the installed binary.",
		},
		{
			name:          "compare unavailable",
			compareStatus: http.StatusNotFound,
			wantHint:      "Could not verify that beta main advances the installed release; keep the installed binary.",
		},
		{
			name:        "compare malformed",
			compareBody: `{"status":`,
			wantHint:    "Could not verify that beta main advances the installed release; keep the installed binary.",
		},
		{
			name:        "compare contradicts its status",
			compareBody: `{"status":"ahead","ahead_by":1,"behind_by":1}`,
			wantHint:    "Could not verify that beta main advances the installed release; keep the installed binary.",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			remote := fakeBetaReleaseRemote(t)
			remote.tagSHA = "08317b9589598f99ebcc00498c39a60f352f05e4"
			remote.main = "c28788b810d488fe68791321fdbde386b63ccee7"
			remote.compareBody = tc.compareBody
			if tc.compareStatus != 0 {
				remote.compareStatus = tc.compareStatus
			}
			binary, calls := fakeBetaReleaseInstall(t)
			ctx := context.Background()
			profile := linuxProfile()
			profile.GoAvailable = true

			results := update.CheckFiltered(ctx, betaReleaseVersion, profile, []string{"gentle-ai"})
			if len(results) != 1 || results[0].Status != update.VersionUnknown || update.HasUpdates(results) {
				t.Errorf("check = %+v; want no update offer", results)
			}
			assertBetaReleaseCheckOutput(t, results, "main@c28788b810d4", tc.wantHint)
			report := ExecuteWithOptions(ctx, results, profile, t.TempDir(), false, ExecuteOptions{SkipBackup: true})
			if len(report.Results) != 1 || report.Results[0].Status != UpgradeSkipped || report.Results[0].Err != nil {
				t.Errorf("report = %+v; want a skip without an upgrade attempt", report)
			}
			out := RenderUpgradeReport(report)
			if !strings.Contains(out, tc.wantHint) || strings.Contains(out, "go install") {
				t.Errorf("upgrade output = %q; want %q and no install command", out, tc.wantHint)
			}
			if len(*calls) != 0 || remote.compareRequests != 1 {
				t.Errorf("commands = %v, compare requests = %d; want no commands and one comparison", *calls, remote.compareRequests)
			}
			assertBetaReleaseBinary(t, binary, betaReleaseVersion)
		})
	}
}

func TestBetaReleaseUnresolvedTagPrintsPinnedRecoveryWithoutInstalling(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		sha    string
	}{
		{name: "missing tag", status: http.StatusNotFound},
		{name: "truncated commit", status: http.StatusOK, sha: "0123456789ab"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			remote := fakeBetaReleaseRemote(t)
			remote.main, remote.tagStatus, remote.tagSHA = betaNextCommit, tc.status, tc.sha
			binary, calls := fakeBetaReleaseInstall(t)
			ctx := context.Background()
			profile := linuxProfile()
			profile.GoAvailable = true
			home := t.TempDir()
			originalSnapshot := snapshotCreator
			t.Cleanup(func() { snapshotCreator = originalSnapshot })
			snapshots := 0
			snapshotCreator = func(string, []string) (backup.Manifest, error) {
				snapshots++
				return backup.Manifest{}, fmt.Errorf("unexpected snapshot")
			}

			results := update.CheckFiltered(ctx, betaReleaseVersion, profile, []string{"gentle-ai"})
			if len(results) != 1 || results[0].Status != update.VersionUnknown || results[0].Err != nil {
				t.Errorf("check = %+v, want unknown (not a failed remote check)", results)
			}
			hint := "go install " + betaReleaseModule + "/cmd/gentle-ai@" + betaNextCommit
			assertBetaReleaseCheckOutput(t, results, "main@89abcdef0123", hint)
			report := Execute(ctx, results, profile, home, false)
			if len(report.Results) != 1 || report.Results[0].Status != UpgradeSkipped || report.Results[0].Err != nil || report.BackupID != "" {
				t.Errorf("report = %+v, want a manual skip with no backup", report)
			}
			want := "Upgrade\n=======\n\n" +
				"  Upgrades managed tool binaries only.\n" +
				"  Agent configs are preserved — no install or sync is performed.\n\n" +
				"  [--] gentle-ai     manual update required: " + hint + "\n\n" +
				"  0 succeeded, 0 failed, 1 skipped.\n"
			if got := RenderUpgradeReport(report); got != want {
				t.Errorf("upgrade output = %q, want %q", got, want)
			}
			if len(*calls) != 0 || snapshots != 0 {
				t.Errorf("commands = %v, snapshots = %d; want neither", *calls, snapshots)
			}
			if remote.tagRequests != 1 || remote.compareRequests != 0 {
				t.Errorf("tag requests = %d, compare requests = %d; want one attempted resolution and no comparison", remote.tagRequests, remote.compareRequests)
			}
			assertBetaReleaseBinary(t, binary, betaReleaseVersion)
		})
	}
}

func assertBetaReleaseCheckOutput(t *testing.T, results []update.UpdateResult, latest, hint string) {
	t.Helper()
	want := "Update Check\n============\n\n" +
		"  [??] gentle-ai     installed: 4.6.0       latest: " + latest + "  " + hint + "\n\n" +
		"Update state could not be determined for 1 tool(s).\n"
	if got := update.RenderCLI(results); got != want {
		t.Errorf("update output = %q, want %q", got, want)
	}
}

// The update HTTP client uses http.DefaultTransport unless a test replaces it.
// This fixture keeps the public check and upgrade flow entirely off the network.
type betaReleaseRemote struct {
	main, tagSHA, compareBody    string
	tagStatus, compareStatus     int
	tagRequests, compareRequests int
}

func fakeBetaReleaseRemote(t *testing.T) *betaReleaseRemote {
	t.Helper()
	t.Setenv("GENTLE_AI_CHANNEL", "beta")
	t.Setenv("GITHUB_TOKEN", "local-test-token")
	remote := &betaReleaseRemote{
		main: betaReleaseCommit, tagSHA: betaReleaseCommit, tagStatus: http.StatusOK,
		compareBody: `{"status":"ahead","ahead_by":1,"behind_by":0}`, compareStatus: http.StatusOK,
	}
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	http.DefaultTransport = remote
	return remote
}

func (r *betaReleaseRemote) RoundTrip(req *http.Request) (*http.Response, error) {
	const api = "api.github.com/repos/Gentleman-Programming/gentle-ai/"
	const raw = "raw.githubusercontent.com/Gentleman-Programming/gentle-ai/"
	status, body := http.StatusOK, ""
	switch req.URL.Host + req.URL.Path {
	case api + "commits/main":
		body = fmt.Sprintf(`{"sha":%q}`, r.main)
	case api + "commits/refs/tags/v4.6.0":
		r.tagRequests++
		status, body = r.tagStatus, fmt.Sprintf(`{"sha":%q}`, r.tagSHA)
	case api + "compare/" + r.tagSHA + "..." + r.main:
		r.compareRequests++
		status, body = r.compareStatus, r.compareBody
	case raw + r.main + "/go.mod":
		body = "module " + betaReleaseModule + "\n"
	default:
		return nil, fmt.Errorf("unexpected request: %s", req.URL)
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
}

func fakeBetaReleaseInstall(t *testing.T) (string, *[]string) {
	t.Helper()
	dir := t.TempDir()
	binary := filepath.Join(dir, "gentle-ai")
	if err := os.WriteFile(binary, []byte(betaReleaseVersion), 0o755); err != nil {
		t.Fatal(err)
	}
	stubDetectOS(t, "linux")
	originalExec, originalLookPath := execCommand, lookPathFn
	t.Cleanup(func() { execCommand, lookPathFn = originalExec, originalLookPath })
	lookPathFn = func(string) (string, error) { return binary, nil }
	var calls []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		call := strings.Join(append([]string{name}, args...), " ")
		calls = append(calls, call)
		switch call {
		case "go env GOBIN":
			return mockCmd("echo", dir)
		case "go install " + betaReleaseModule + "/cmd/gentle-ai@" + betaNextCommit:
			if err := os.WriteFile(binary, []byte(betaNextCommit), 0o755); err != nil {
				t.Fatal(err)
			}
			return mockCmd("true")
		default:
			t.Errorf("unexpected command: %s", call)
			return mockCmd("false")
		}
	}
	return binary, &calls
}

func assertBetaReleaseBinary(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Errorf("fake binary = %q, %v; want %q", got, err, want)
	}
}

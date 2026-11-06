package telemetrycollector

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// testServerNextPageURL lets a pagination test's first-page handler
// advertise a Link: rel="next" URL that depends on the httptest server's
// own address, which is only known after httptest.NewServer returns (i.e.
// after the handler closures below are already registered).
var testServerNextPageURL string

// stubDownloadsAPIs points npmDownloadsURL/githubReleasesURL at httptest
// servers for the duration of the test, restoring the real endpoints
// afterward.
func stubDownloadsAPIs(t *testing.T, npmHandler, githubHandler http.HandlerFunc) {
	t.Helper()
	origNpm, origGithub := npmDownloadsURL, githubReleasesURL
	t.Cleanup(func() {
		npmDownloadsURL, githubReleasesURL = origNpm, origGithub
	})

	if npmHandler != nil {
		npmServer := httptest.NewServer(npmHandler)
		t.Cleanup(npmServer.Close)
		npmDownloadsURL = func(pkg string) string { return npmServer.URL + "/" + pkg }
	}
	if githubHandler != nil {
		githubServer := httptest.NewServer(githubHandler)
		t.Cleanup(githubServer.Close)
		githubReleasesURL = func(repo string) string { return githubServer.URL + "/" + repo }
	}
}

func TestFetchAndStoreDownloads_WritesExpectedRows(t *testing.T) {
	s := openTestStorage(t)
	ctx := context.Background()

	stubDownloadsAPIs(t,
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch r.URL.Path {
			case "/gentle-pi":
				w.Write([]byte(`{"downloads":42,"package":"gentle-pi"}`))
			case "/gentle-engram":
				w.Write([]byte(`{"downloads":7,"package":"gentle-engram"}`))
			default:
				w.Write([]byte(`{"error":"package not found"}`))
			}
		},
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[
				{"tag_name":"v2.0.0","assets":[{"download_count":100},{"download_count":50}]},
				{"tag_name":"v1.0.0","assets":[{"download_count":10}]}
			]`))
		},
	)

	now := time.Date(2026, 6, 10, 3, 0, 0, 0, time.UTC)
	cfg := DownloadsConfig{
		NpmPackages: []string{"gentle-pi", "gentle-engram"},
		GithubRepos: []string{"Gentleman-Programming/gentle-ai"},
	}
	FetchAndStoreDownloads(ctx, s, http.DefaultClient, cfg, now, nil)

	npmDay := now.AddDate(0, 0, -1).Format(dayLayout)
	npmRows, err := s.rollupRows(ctx, metricNpmDownloadsDay, npmDay, npmDay)
	if err != nil {
		t.Fatalf("rollupRows(npm): %v", err)
	}
	npmByKey := map[string]int64{}
	for _, r := range npmRows {
		npmByKey[r.Key] = r.Value
	}
	if npmByKey["gentle-pi"] != 42 {
		t.Errorf("gentle-pi npm downloads = %d, want 42", npmByKey["gentle-pi"])
	}
	if npmByKey["gentle-engram"] != 7 {
		t.Errorf("gentle-engram npm downloads = %d, want 7", npmByKey["gentle-engram"])
	}

	githubDay := now.Format(dayLayout)
	githubRows, err := s.rollupRows(ctx, metricGithubReleaseDownloads, githubDay, githubDay)
	if err != nil {
		t.Fatalf("rollupRows(github): %v", err)
	}
	githubByKey := map[string]int64{}
	for _, r := range githubRows {
		githubByKey[r.Key] = r.Value
	}
	if githubByKey["v2.0.0"] != 150 {
		t.Errorf("v2.0.0 total = %d, want 150 (100+50)", githubByKey["v2.0.0"])
	}
	if githubByKey["v1.0.0"] != 10 {
		t.Errorf("v1.0.0 total = %d, want 10", githubByKey["v1.0.0"])
	}
}

// TestFetchGithubReleaseDownloads_FollowsLinkHeaderPagination serves two
// pages of releases linked via the Link header's rel="next" and asserts
// both pages' releases are aggregated into one result, and that the first
// request asks for per_page=100.
func TestFetchGithubReleaseDownloads_FollowsLinkHeaderPagination(t *testing.T) {
	origGithub := githubReleasesURL
	t.Cleanup(func() { githubReleasesURL = origGithub })

	var requestedPaths []string
	mux := http.NewServeMux()
	mux.HandleFunc("/octo/repo/releases", func(w http.ResponseWriter, r *http.Request) {
		requestedPaths = append(requestedPaths, r.URL.RequestURI())
		if r.URL.Query().Get("per_page") != "100" {
			t.Errorf("first page request per_page = %q, want 100", r.URL.Query().Get("per_page"))
		}
		w.Header().Set("Link", `<`+testServerNextPageURL+`>; rel="next", <`+testServerNextPageURL+`>; rel="last"`)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"tag_name":"v2.0.0","assets":[{"download_count":100}]}]`))
	})
	mux.HandleFunc("/octo/repo/releases/page2", func(w http.ResponseWriter, r *http.Request) {
		requestedPaths = append(requestedPaths, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"tag_name":"v1.0.0","assets":[{"download_count":10}]}]`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	testServerNextPageURL = server.URL + "/octo/repo/releases/page2"
	githubReleasesURL = func(repo string) string { return server.URL + "/" + repo + "/releases?per_page=100" }

	totals, err := fetchGithubReleaseDownloads(context.Background(), http.DefaultClient, "octo/repo", "")
	if err != nil {
		t.Fatalf("fetchGithubReleaseDownloads: %v", err)
	}
	if totals["v2.0.0"] != 100 {
		t.Errorf("v2.0.0 (page 1) = %d, want 100", totals["v2.0.0"])
	}
	if totals["v1.0.0"] != 10 {
		t.Errorf("v1.0.0 (page 2) = %d, want 10", totals["v1.0.0"])
	}
	if len(requestedPaths) != 2 {
		t.Fatalf("requested %d pages, want 2: %v", len(requestedPaths), requestedPaths)
	}
}

// TestFetchGithubReleaseDownloads_StopsAtTenPages guards against an
// unbounded Link-header loop (e.g. a server that always links to a
// "next" page): pagination must stop after 10 pages even if the server
// keeps offering another one.
func TestFetchGithubReleaseDownloads_StopsAtTenPages(t *testing.T) {
	origGithub := githubReleasesURL
	t.Cleanup(func() { githubReleasesURL = origGithub })

	var requests int
	mux := http.NewServeMux()
	mux.HandleFunc("/octo/repo/releases", func(w http.ResponseWriter, r *http.Request) {
		requests++
		page := requests
		w.Header().Set("Link", fmt.Sprintf(`<http://%s/octo/repo/releases?page=%d>; rel="next"`, r.Host, page+1))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `[{"tag_name":"v%d","assets":[{"download_count":1}]}]`, page)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	githubReleasesURL = func(repo string) string { return server.URL + "/" + repo + "/releases" }

	totals, err := fetchGithubReleaseDownloads(context.Background(), http.DefaultClient, "octo/repo", "")
	if err != nil {
		t.Fatalf("fetchGithubReleaseDownloads: %v", err)
	}
	if requests != 10 {
		t.Fatalf("requests = %d, want exactly 10 (the page cap)", requests)
	}
	if len(totals) != 10 {
		t.Fatalf("aggregated releases = %d, want 10", len(totals))
	}
}

// TestFetchGithubReleaseDownloads_StopsOnLaterPageErrorButKeepsEarlierData
// asserts a failure fetching a later page stops pagination without
// discarding the releases already fetched from earlier pages.
func TestFetchGithubReleaseDownloads_StopsOnLaterPageErrorButKeepsEarlierData(t *testing.T) {
	origGithub := githubReleasesURL
	t.Cleanup(func() { githubReleasesURL = origGithub })

	mux := http.NewServeMux()
	mux.HandleFunc("/octo/repo/releases", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", `<`+testServerNextPageURL+`>; rel="next"`)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"tag_name":"v1.0.0","assets":[{"download_count":5}]}]`))
	})
	mux.HandleFunc("/octo/repo/releases/page2", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	testServerNextPageURL = server.URL + "/octo/repo/releases/page2"
	githubReleasesURL = func(repo string) string { return server.URL + "/" + repo + "/releases" }

	totals, err := fetchGithubReleaseDownloads(context.Background(), http.DefaultClient, "octo/repo", "")
	if err != nil {
		t.Fatalf("fetchGithubReleaseDownloads: %v, want a nil error with the earlier page's data preserved", err)
	}
	if totals["v1.0.0"] != 5 {
		t.Errorf("v1.0.0 (page 1) = %d, want 5 to survive the page 2 failure", totals["v1.0.0"])
	}
	if len(totals) != 1 {
		t.Errorf("totals = %+v, want only page 1's release", totals)
	}
}

// TestFetchGithubReleaseDownloads_StopsWhenContextDeadlineIsNear asserts
// pagination stops before issuing a next-page request once the context's
// remaining time is too small to be worth starting it, preserving
// whatever was already fetched instead of erroring.
func TestFetchGithubReleaseDownloads_StopsWhenContextDeadlineIsNear(t *testing.T) {
	origGithub := githubReleasesURL
	t.Cleanup(func() { githubReleasesURL = origGithub })

	var page2Requested bool
	mux := http.NewServeMux()
	mux.HandleFunc("/octo/repo/releases", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", `<`+testServerNextPageURL+`>; rel="next"`)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"tag_name":"v1.0.0","assets":[{"download_count":5}]}]`))
	})
	mux.HandleFunc("/octo/repo/releases/page2", func(w http.ResponseWriter, r *http.Request) {
		page2Requested = true
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"tag_name":"v2.0.0","assets":[{"download_count":9}]}]`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	testServerNextPageURL = server.URL + "/octo/repo/releases/page2"
	githubReleasesURL = func(repo string) string { return server.URL + "/" + repo + "/releases" }

	// A deadline shorter than githubPaginationDeadlineBuffer: the first
	// page still gets fetched (no deadline check before it), but the loop
	// must stop before requesting page 2.
	ctx, cancel := context.WithTimeout(context.Background(), githubPaginationDeadlineBuffer/2)
	defer cancel()

	totals, err := fetchGithubReleaseDownloads(ctx, http.DefaultClient, "octo/repo", "")
	if err != nil {
		t.Fatalf("fetchGithubReleaseDownloads: %v", err)
	}
	if page2Requested {
		t.Fatal("page 2 was requested despite the near context deadline")
	}
	if totals["v1.0.0"] != 5 || len(totals) != 1 {
		t.Errorf("totals = %+v, want only page 1's release", totals)
	}
}

func TestFetchAndStoreDownloads_FailingAPILeavesPriorRowsIntact(t *testing.T) {
	s := openTestStorage(t)
	ctx := context.Background()

	// First run: both APIs succeed.
	stubDownloadsAPIs(t,
		func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"downloads":5}`))
		},
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"tag_name":"v1.0.0","assets":[{"download_count":3}]}]`))
		},
	)
	day1 := time.Date(2026, 6, 10, 3, 0, 0, 0, time.UTC)
	cfg := DownloadsConfig{NpmPackages: []string{"gentle-pi"}, GithubRepos: []string{"Gentleman-Programming/gentle-ai"}}
	FetchAndStoreDownloads(ctx, s, http.DefaultClient, cfg, day1, nil)

	// Second run, a day later: npm now fails (500); github still succeeds
	// with a higher count. The failing npm fetch must not touch the row
	// written on day1, and must not stop the github fetch from writing.
	stubDownloadsAPIs(t,
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		},
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"tag_name":"v1.0.0","assets":[{"download_count":9}]}]`))
		},
	)
	day2 := day1.AddDate(0, 0, 1)
	FetchAndStoreDownloads(ctx, s, http.DefaultClient, cfg, day2, nil)

	npmDay1 := day1.AddDate(0, 0, -1).Format(dayLayout)
	rows, err := s.rollupRows(ctx, metricNpmDownloadsDay, npmDay1, npmDay1)
	if err != nil {
		t.Fatalf("rollupRows: %v", err)
	}
	if len(rows) != 1 || rows[0].Value != 5 {
		t.Errorf("day1's npm row should survive the day2 failure untouched, got %+v", rows)
	}

	npmDay2 := day2.AddDate(0, 0, -1).Format(dayLayout)
	rows2, err := s.rollupRows(ctx, metricNpmDownloadsDay, npmDay2, npmDay2)
	if err != nil {
		t.Fatalf("rollupRows: %v", err)
	}
	if len(rows2) != 0 {
		t.Errorf("a failing fetch must not write a row for day2, got %+v", rows2)
	}

	githubLatest, err := s.latestRollupPerKey(ctx, metricGithubReleaseDownloads, day2.Format(dayLayout))
	if err != nil {
		t.Fatalf("latestRollupPerKey: %v", err)
	}
	if githubLatest["v1.0.0"] != 9 {
		t.Errorf("github fetch should still have succeeded on day2: v1.0.0 = %d, want 9", githubLatest["v1.0.0"])
	}
}

func TestBuildSummary_IncludesDownloads(t *testing.T) {
	s := openTestStorage(t)
	ctx := context.Background()

	stubDownloadsAPIs(t,
		func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"downloads":20}`))
		},
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"tag_name":"v1.0.0","assets":[{"download_count":30}]}]`))
		},
	)

	day1 := time.Date(2026, 6, 10, 3, 0, 0, 0, time.UTC)
	cfg := DownloadsConfig{NpmPackages: []string{"gentle-pi"}, GithubRepos: []string{"Gentleman-Programming/gentle-ai"}}
	FetchAndStoreDownloads(ctx, s, http.DefaultClient, cfg, day1, nil)

	day2 := day1.AddDate(0, 0, 1)
	stubDownloadsAPIs(t,
		func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"downloads":25}`))
		},
		nil,
	)
	FetchAndStoreDownloads(ctx, s, http.DefaultClient, cfg, day2, nil)

	summary, err := BuildSummary(ctx, s, day2)
	if err != nil {
		t.Fatalf("BuildSummary: %v", err)
	}

	npm := summary.Downloads.Npm["gentle-pi"]
	if npm.LastDay != 25 {
		t.Errorf("npm gentle-pi last_day = %d, want 25", npm.LastDay)
	}
	if npm.Last30Days != 45 {
		t.Errorf("npm gentle-pi last_30_days = %d, want 45 (20+25)", npm.Last30Days)
	}
	if summary.Downloads.Github["v1.0.0"] != 30 {
		t.Errorf("github v1.0.0 total = %d, want 30", summary.Downloads.Github["v1.0.0"])
	}
}

// TestRunMaintenance_DownloadRowsDoNotPoisonRollupCatchUp exercises the
// exact sequence main.go's runOnce runs every cycle: FetchAndStoreDownloads
// followed by RunMaintenance. Download rows must not be mistaken for the
// telemetry watermark (they can land before any telemetry rollup ever
// runs), and a day already carrying a download row must keep it once that
// day is (re-)rolled up.
func TestRunMaintenance_DownloadRowsDoNotPoisonRollupCatchUp(t *testing.T) {
	s := openTestStorage(t)
	ctx := context.Background()

	day1 := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC)
	day3 := time.Date(2026, 6, 3, 9, 0, 0, 0, time.UTC)
	now := time.Date(2026, 6, 4, 3, 0, 0, 0, time.UTC) // yesterday = day3

	for day, ev := range map[time.Time]Event{day1: installEvent(t, installA), day2: installEvent(t, installB), day3: installEvent(t, installC)} {
		if err := s.InsertEvent(ctx, ev, day); err != nil {
			t.Fatalf("InsertEvent(%s): %v", day.Format(dayLayout), err)
		}
	}

	// downloadsNow=day2 stores the npm row under day1 (npm reports the
	// prior day) and the github row under day2 itself — both before any
	// telemetry rollup has ever run.
	stubDownloadsAPIs(t,
		func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"downloads":11}`)) },
		func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`[{"tag_name":"v1.0.0","assets":[{"download_count":22}]}]`))
		},
	)
	cfg := DownloadsConfig{NpmPackages: []string{"gentle-pi"}, GithubRepos: []string{"Gentleman-Programming/gentle-ai"}}
	FetchAndStoreDownloads(ctx, s, http.DefaultClient, cfg, day2, nil)

	if err := RunMaintenance(ctx, s, now, 90, 2); err != nil {
		t.Fatalf("RunMaintenance: %v", err)
	}

	for day, installID := range map[time.Time]string{day1: installA, day2: installB, day3: installC} {
		rows, err := s.rollupRows(ctx, metricActiveInstall, day.Format(dayLayout), day.Format(dayLayout))
		if err != nil {
			t.Fatalf("rollupRows(%s): %v", day.Format(dayLayout), err)
		}
		if len(rows) != 1 || rows[0].Key != installID {
			t.Errorf("day %s telemetry rollup = %+v, want one row for %s (the download rows must not stall catch-up)", day.Format(dayLayout), rows, installID)
		}
	}

	npmRows, err := s.rollupRows(ctx, metricNpmDownloadsDay, day1.Format(dayLayout), day1.Format(dayLayout))
	if err != nil {
		t.Fatalf("rollupRows(npm): %v", err)
	}
	if len(npmRows) != 1 || npmRows[0].Value != 11 {
		t.Errorf("day1's npm download row should survive being (re-)rolled up, got %+v", npmRows)
	}

	githubRows, err := s.rollupRows(ctx, metricGithubReleaseDownloads, day2.Format(dayLayout), day2.Format(dayLayout))
	if err != nil {
		t.Fatalf("rollupRows(github): %v", err)
	}
	if len(githubRows) != 1 || githubRows[0].Value != 22 {
		t.Errorf("day2's github download row should survive being (re-)rolled up, got %+v", githubRows)
	}
}

// TestFetchAndStoreDownloads_CancelledContextSkipsStorage guards the
// shutdown fix in cmd/gentle-telemetry: an already-cancelled context must
// make the fetch return promptly, and store nothing, rather than run to
// completion.
func TestFetchAndStoreDownloads_CancelledContextSkipsStorage(t *testing.T) {
	s := openTestStorage(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stubDownloadsAPIs(t,
		func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"downloads":99}`)) },
		func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`[{"tag_name":"v1.0.0","assets":[{"download_count":99}]}]`))
		},
	)

	now := time.Date(2026, 6, 10, 3, 0, 0, 0, time.UTC)
	cfg := DownloadsConfig{NpmPackages: []string{"gentle-pi"}, GithubRepos: []string{"Gentleman-Programming/gentle-ai"}}

	done := make(chan struct{})
	go func() {
		FetchAndStoreDownloads(ctx, s, http.DefaultClient, cfg, now, nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("FetchAndStoreDownloads did not return promptly for an already-cancelled context")
	}

	bg := context.Background()
	npmDay := now.AddDate(0, 0, -1).Format(dayLayout)
	rows, err := s.rollupRows(bg, metricNpmDownloadsDay, npmDay, npmDay)
	if err != nil {
		t.Fatalf("rollupRows: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("a cancelled context must not store any downloads row, got %+v", rows)
	}
}

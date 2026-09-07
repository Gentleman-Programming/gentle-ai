// Command gentle-telemetry is the self-hosted collector for gentle-ai's
// anonymous telemetry events. See docs/telemetry-collector.md for the
// deploy story and docs/telemetry-collector.md#answering-how-many-people-use-it
// for how to read /v1/summary.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/telemetrycollector"
)

const (
	defaultListen           = "127.0.0.1:18181"
	defaultDB               = "/var/lib/gentle-telemetry/events.sqlite"
	defaultRetentionDays    = 90
	defaultRateLimitPerMin  = 60
	maintenanceInterval     = 24 * time.Hour
	rateLimiterSweepEvery   = 10 * time.Minute
	rateLimiterSweepMaxIdle = 30 * time.Minute
	shutdownTimeout         = 10 * time.Second
	// downloadsTimeout bounds one FetchAndStoreDownloads run, well under
	// the unit's TimeoutStopSec=15s, so a slow npm/GitHub endpoint can
	// never block graceful shutdown.
	downloadsTimeout = 8 * time.Second
)

// repeatableFlag collects a repeatable flag (--trusted-proxy-cidr,
// --npm-package, --github-repo) into a []string.
type repeatableFlag []string

func (f *repeatableFlag) String() string { return strings.Join(*f, ",") }
func (f *repeatableFlag) Set(v string) error {
	*f = append(*f, v)
	return nil
}

func parseCIDRs(values []string) ([]*net.IPNet, error) {
	nets := make([]*net.IPNet, 0, len(values))
	for _, v := range values {
		_, ipnet, err := net.ParseCIDR(v)
		if err != nil {
			return nil, fmt.Errorf("%q: %w", v, err)
		}
		nets = append(nets, ipnet)
	}
	return nets, nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "gentle-telemetry: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	listen := flag.String("listen", defaultListen, "address to listen on for HTTP")
	dbPath := flag.String("db", defaultDB, "path to the SQLite database file")
	summaryTokenFile := flag.String("summary-token-file", "", "path to a file containing the bearer token required for GET /v1/summary")
	retentionDays := flag.Int("retention-days", defaultRetentionDays, "days of raw events to retain before purge")
	rateLimitPerMinute := flag.Int("rate-limit-per-minute", defaultRateLimitPerMin, "per-IP request budget for POST /v1/events, per minute")
	var trustedProxyCIDRs repeatableFlag
	flag.Var(&trustedProxyCIDRs, "trusted-proxy-cidr", "CIDR of a peer allowed to set X-Forwarded-For/X-Real-IP for rate limiting (repeatable; default 127.0.0.0/8,::1/128)")
	var npmPackages repeatableFlag
	flag.Var(&npmPackages, "npm-package", "npm package to fetch daily download counts for (repeatable; default gentle-pi,gentle-engram)")
	var githubRepos repeatableFlag
	flag.Var(&githubRepos, "github-repo", "GitHub owner/repo to fetch release download counts for (repeatable; default Gentleman-Programming/gentle-ai)")
	githubTokenFile := flag.String("github-token-file", "", "path to a file containing a GitHub token, to raise the API rate limit for --github-repo")
	flag.Parse()

	if len(trustedProxyCIDRs) == 0 {
		trustedProxyCIDRs = repeatableFlag{"127.0.0.0/8", "::1/128"} // matches Apache on loopback
	}
	trustedProxies, err := parseCIDRs(trustedProxyCIDRs)
	if err != nil {
		return fmt.Errorf("parse --trusted-proxy-cidr: %w", err)
	}
	if len(npmPackages) == 0 {
		npmPackages = repeatableFlag{"gentle-pi", "gentle-engram"}
	}
	if len(githubRepos) == 0 {
		githubRepos = repeatableFlag{"Gentleman-Programming/gentle-ai"}
	}
	var githubToken string
	if *githubTokenFile != "" {
		data, err := os.ReadFile(*githubTokenFile)
		if err != nil {
			return fmt.Errorf("load github token: %w", err)
		}
		githubToken = strings.TrimSpace(string(data))
	}
	downloadsCfg := telemetrycollector.DownloadsConfig{
		NpmPackages: npmPackages,
		GithubRepos: githubRepos,
		GithubToken: githubToken,
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	summaryToken, err := loadSummaryToken(*summaryTokenFile)
	if err != nil {
		return fmt.Errorf("load summary token: %w", err)
	}
	if summaryToken == "" {
		logger.Warn("no --summary-token-file provided: GET /v1/summary will reject every request")
	}

	if dir := filepath.Dir(*dbPath); dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("create database directory %q: %w", dir, err)
		}
	}

	storage, err := telemetrycollector.OpenStorage(*dbPath)
	if err != nil {
		return fmt.Errorf("open storage: %w", err)
	}
	defer storage.Close()

	limiter := telemetrycollector.NewRateLimiter(*rateLimitPerMinute)

	server := &telemetrycollector.Server{
		Storage:        storage,
		Limiter:        limiter,
		SummaryToken:   summaryToken,
		Logger:         logger,
		TrustedProxies: trustedProxies,
	}

	httpServer := &http.Server{
		Addr:              *listen,
		Handler:           server.NewMux(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// maintenanceDone is waited on below before storage.Close() runs (via
	// the earlier defer), so an in-flight maintenance run — which holds
	// its own uncancelable context for the day it is committing, see
	// RunMaintenance — is never racing a closed database out from under
	// it.
	downloadsClient := &http.Client{Timeout: 10 * time.Second}

	var maintenanceDone sync.WaitGroup
	maintenanceDone.Add(1)
	go func() {
		defer maintenanceDone.Done()
		runMaintenanceLoop(ctx, storage, limiter, *retentionDays, downloadsCfg, downloadsClient, logger)
	}()

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("gentle-telemetry listening", "addr", *listen, "db", *dbPath)
		serveErr <- httpServer.ListenAndServe()
	}()

	var runErr error
	select {
	case <-ctx.Done():
		logger.Info("shutting down")
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			runErr = fmt.Errorf("http server: %w", err)
		}
	}

	// Cancel ctx unconditionally: the serveErr branch above may not have
	// come from a signal, so runMaintenanceLoop would otherwise never see
	// a cancellation and maintenanceDone.Wait() below would block forever.
	// stop (like any context.CancelFunc) is safe to call more than once.
	stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil && runErr == nil {
		runErr = fmt.Errorf("http server shutdown: %w", err)
	}

	maintenanceDone.Wait()
	return runErr
}

// loadSummaryToken reads the /v1/summary bearer token. When path is empty
// it falls back to systemd LoadCredential (CREDENTIALS_DIRECTORY/summary-token),
// so the source file can stay root-owned 0600 under a DynamicUser service.
func loadSummaryToken(path string) (string, error) {
	if path == "" {
		if credDir := os.Getenv("CREDENTIALS_DIRECTORY"); credDir != "" {
			path = filepath.Join(credDir, "summary-token")
		} else {
			return "", nil
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// runMaintenanceLoop runs the daily rollup+retention job once at startup and
// then on a fixed interval, fetches external npm/GitHub download counts
// right after it (a separate, independent step: a failure there is logged
// and retried the next run, and never affects the rollup or ingest), and
// periodically sweeps idle rate-limiter buckets, until ctx is cancelled.
func runMaintenanceLoop(ctx context.Context, storage *telemetrycollector.Storage, limiter *telemetrycollector.RateLimiter, retentionDays int, downloadsCfg telemetrycollector.DownloadsConfig, downloadsClient *http.Client, logger *slog.Logger) {
	runOnce := func() {
		if err := telemetrycollector.RunMaintenance(ctx, storage, time.Now(), retentionDays); err != nil {
			logger.Error("daily maintenance failed", "error", err)
		}
		// Derived from ctx (not WithoutCancel): a SIGTERM cancels an
		// in-flight fetch immediately instead of running it to
		// completion, and the bounded timeout caps the worst case even
		// absent a signal. A cancelled request simply fails and is
		// logged like any other fetch error; nothing is stored for it.
		downloadsCtx, cancelDownloads := context.WithTimeout(ctx, downloadsTimeout)
		telemetrycollector.FetchAndStoreDownloads(downloadsCtx, storage, downloadsClient, downloadsCfg, time.Now(), logger)
		cancelDownloads()
	}
	runOnce()

	maintenanceTicker := time.NewTicker(maintenanceInterval)
	defer maintenanceTicker.Stop()
	sweepTicker := time.NewTicker(rateLimiterSweepEvery)
	defer sweepTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-maintenanceTicker.C:
			runOnce()
		case <-sweepTicker.C:
			limiter.Sweep(rateLimiterSweepMaxIdle)
		}
	}
}

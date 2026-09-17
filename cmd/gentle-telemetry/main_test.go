package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadSummaryToken(t *testing.T) {
	dir := t.TempDir()
	explicit := filepath.Join(dir, "explicit")
	os.WriteFile(filepath.Join(dir, "summary-token"), []byte(" from-credential \n"), 0o600)
	os.WriteFile(explicit, []byte("from-flag"), 0o600)

	for _, tt := range []struct{ name, path, credDir, want string }{
		{"falls back to CREDENTIALS_DIRECTORY", "", dir, "from-credential"},
		{"explicit path wins", explicit, dir, "from-flag"},
		{"neither configured returns empty", "", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CREDENTIALS_DIRECTORY", tt.credDir)
			if got, err := loadSummaryToken(tt.path); err != nil || got != tt.want {
				t.Errorf("loadSummaryToken(%q) = (%q, %v), want (%q, nil)", tt.path, got, err, tt.want)
			}
		})
	}
}

func TestNextMaintenanceDelay(t *testing.T) {
	for _, tt := range []struct {
		name string
		now  time.Time
		want time.Duration
	}{
		{
			name: "before 00:05 UTC on the same day",
			now:  time.Date(2026, 9, 17, 0, 4, 0, 0, time.UTC),
			want: 1 * time.Minute,
		},
		{
			name: "after 00:05 UTC waits for the next day",
			now:  time.Date(2026, 9, 16, 23, 59, 0, 0, time.UTC),
			want: 6 * time.Minute,
		},
		{
			name: "exactly 00:05:00 UTC waits a full day",
			now:  time.Date(2026, 9, 17, 0, 5, 0, 0, time.UTC),
			want: 24 * time.Hour,
		},
		{
			name: "non-UTC input is converted before comparing",
			// 12:00 in UTC-3 is 15:00 UTC, well past 00:05 UTC that day.
			now:  time.Date(2026, 9, 17, 12, 0, 0, 0, time.FixedZone("UTC-3", -3*60*60)),
			want: (24*time.Hour - 15*time.Hour + 5*time.Minute),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := nextMaintenanceDelay(tt.now)
			if got != tt.want {
				t.Errorf("nextMaintenanceDelay(%v) = %v, want %v", tt.now, got, tt.want)
			}
			if got <= 0 || got > 24*time.Hour {
				t.Errorf("nextMaintenanceDelay(%v) = %v, want in (0, 24h]", tt.now, got)
			}
		})
	}
}

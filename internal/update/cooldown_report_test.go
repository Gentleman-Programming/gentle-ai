package update

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

func TestCooldownReportProvenance(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ttl     time.Duration
		results []UpdateResult
		want    CheckState
		calls   int
	}{
		{"skipped", UpdateCheckTTL, nil, CheckSkipped, 0},
		{"forced-current", 0, []UpdateResult{{Status: UpToDate}}, CheckCompleted, 1},
		{"forced-failure", 0, []UpdateResult{{Status: CheckFailed}}, CheckUnsuccessful, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			now := time.Now()
			recent := now.Add(-time.Minute)
			if err := state.Write(home, state.InstallState{LastUpdateCheck: &recent}); err != nil {
				t.Fatal(err)
			}
			calls := 0
			report := CheckAllWithCooldownReport(context.Background(), "1.0.0", system.PlatformProfile{}, home, tc.ttl, func() time.Time { return now }, func(context.Context, string, system.PlatformProfile) []UpdateResult { calls++; return tc.results })
			if report.State != tc.want || calls != tc.calls || !reflect.DeepEqual(report.Results, tc.results) {
				t.Fatalf("report=%+v calls=%d", report, calls)
			}
			saved, err := state.Read(home)
			wantTime := recent
			if tc.want == CheckCompleted {
				wantTime = now
			}
			if err != nil || saved.LastUpdateCheck == nil || !saved.LastUpdateCheck.Equal(wantTime) {
				t.Fatalf("timestamp=%+v err=%v", saved, err)
			}
		})
	}
}

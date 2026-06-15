package discovery_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/components/discovery"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// --- Mock source ---

// mockSource is a test double for SourceReader.
type mockSource struct {
	name    string
	results []model.MCPServerRef
	err     error
}

func (m *mockSource) Name() string { return m.name }

func (m *mockSource) Search(_ context.Context, _ discovery.DiscoveryQuery) ([]model.MCPServerRef, error) {
	return m.results, m.err
}

func (m *mockSource) HealthCheck(_ context.Context) error { return nil }

// --- Scorer tests ---

func TestQualityScorerRankEmpty(t *testing.T) {
	scorer := discovery.NewQualityScorer()
	got := scorer.Rank(nil)
	if len(got) != 0 {
		t.Errorf("Rank(nil) returned %d items, want 0", len(got))
	}
}

func TestQualityScorerRankSortsDescending(t *testing.T) {
	scorer := discovery.NewQualityScorer()
	input := []model.MCPServerRef{
		{Name: "low", QualityScore: 30},
		{Name: "high", QualityScore: 90},
		{Name: "mid", QualityScore: 60},
	}

	ranked := scorer.Rank(input)
	if len(ranked) != 3 {
		t.Fatalf("Rank returned %d items, want 3", len(ranked))
	}
	if ranked[0].Name != "high" {
		t.Errorf("first ranked = %q, want %q", ranked[0].Name, "high")
	}
	if ranked[1].Name != "mid" {
		t.Errorf("second ranked = %q, want %q", ranked[1].Name, "mid")
	}
	if ranked[2].Name != "low" {
		t.Errorf("third ranked = %q, want %q", ranked[2].Name, "low")
	}
}

func TestQualityScorerRankPreservesOrderOnEqual(t *testing.T) {
	scorer := discovery.NewQualityScorer()
	input := []model.MCPServerRef{
		{Name: "a", QualityScore: 50},
		{Name: "b", QualityScore: 50},
	}

	ranked := scorer.Rank(input)
	if len(ranked) != 2 {
		t.Fatalf("Rank returned %d items, want 2", len(ranked))
	}
	// Equal scores should remain in original order (stable sort)
	if ranked[0].Name != "a" || ranked[1].Name != "b" {
		t.Errorf("expected stable order [a,b], got [%s,%s]", ranked[0].Name, ranked[1].Name)
	}
}

// --- Dedup tests ---

func TestDeduplicatorMergeByNameAndURL(t *testing.T) {
	dedup := discovery.NewDeduplicator()
	input := []model.MCPServerRef{
		{Name: "nuclei", URL: "https://github.com/nuclei", QualityScore: 80},
		{Name: "nuclei", URL: "https://github.com/nuclei", QualityScore: 95},
		{Name: "semgrep", URL: "https://github.com/semgrep", QualityScore: 70},
	}

	deduped := dedup.Deduplicate(input)
	if len(deduped) != 2 {
		t.Fatalf("Deduplicate returned %d items, want 2", len(deduped))
	}

	// The higher-scored nuclei should survive
	for _, item := range deduped {
		if item.Name == "nuclei" && item.QualityScore != 95 {
			t.Errorf("nuclei score = %v, want 95 (highest kept)", item.QualityScore)
		}
	}
}

func TestDeduplicatorKeepsDistinctEntries(t *testing.T) {
	dedup := discovery.NewDeduplicator()
	input := []model.MCPServerRef{
		{Name: "tool-a", URL: "https://a.com", QualityScore: 50},
		{Name: "tool-b", URL: "https://b.com", QualityScore: 60},
		{Name: "tool-c", URL: "https://c.com", QualityScore: 70},
	}

	deduped := dedup.Deduplicate(input)
	if len(deduped) != 3 {
		t.Fatalf("Deduplicate returned %d items, want 3", len(deduped))
	}
}

func TestDeduplicatorEmptyInput(t *testing.T) {
	dedup := discovery.NewDeduplicator()
	got := dedup.Deduplicate(nil)
	if len(got) != 0 {
		t.Errorf("Deduplicate(nil) returned %d items, want 0", len(got))
	}
}

// --- Engine tests ---

func TestDiscoveryEngineSearchAggregatesSources(t *testing.T) {
	src1 := &mockSource{
		name: "source-a",
		results: []model.MCPServerRef{
			{Name: "tool-1", QualityScore: 80},
		},
	}
	src2 := &mockSource{
		name: "source-b",
		results: []model.MCPServerRef{
			{Name: "tool-2", QualityScore: 90},
		},
	}

	engine := discovery.NewEngine([]discovery.SourceReader{src1, src2})
	result, err := engine.Search(context.Background(), discovery.DiscoveryQuery{
		Role: model.RoleCybersecurity,
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(result.MCPServers) != 2 {
		t.Errorf("expected 2 MCP servers, got %d", len(result.MCPServers))
	}
}

func TestDiscoveryEngineSearchGracefulDegradation(t *testing.T) {
	srcOK := &mockSource{
		name: "working-source",
		results: []model.MCPServerRef{
			{Name: "tool-ok", QualityScore: 85},
		},
	}
	srcFail := &mockSource{
		name: "failing-source",
		err:  errors.New("connection refused"),
	}

	engine := discovery.NewEngine([]discovery.SourceReader{srcOK, srcFail})
	result, err := engine.Search(context.Background(), discovery.DiscoveryQuery{
		Role: model.RoleDeveloper,
	})
	if err != nil {
		t.Fatalf("Search returned error (should degrade gracefully): %v", err)
	}
	if len(result.MCPServers) != 1 {
		t.Errorf("expected 1 MCP server from working source, got %d", len(result.MCPServers))
	}
}

func TestDiscoveryEngineSearchAllSourcesFail(t *testing.T) {
	src1 := &mockSource{name: "a", err: errors.New("timeout")}
	src2 := &mockSource{name: "b", err: errors.New("503")}

	engine := discovery.NewEngine([]discovery.SourceReader{src1, src2})
	_, err := engine.Search(context.Background(), discovery.DiscoveryQuery{
		Role: model.RoleMarketing,
	})
	if err == nil {
		t.Fatal("expected error when all sources fail, got nil")
	}
}

func TestDiscoveryEngineSearchEmptySources(t *testing.T) {
	engine := discovery.NewEngine(nil)
	result, err := engine.Search(context.Background(), discovery.DiscoveryQuery{
		Role: model.RoleEducation,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.MCPServers) != 0 {
		t.Errorf("expected 0 MCP servers with no sources, got %d", len(result.MCPServers))
	}
}

func TestDiscoveryEngineSearchRespectsContextTimeout(t *testing.T) {
	src := &slowSource{delay: 5 * time.Second}
	engine := discovery.NewEngine([]discovery.SourceReader{src})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := engine.Search(ctx, discovery.DiscoveryQuery{
		Role: model.RoleDesign,
	})
	if err == nil {
		t.Fatal("expected error for context timeout, got nil")
	}
}

// slowSource simulates a source that takes a long time.
type slowSource struct {
	delay time.Duration
}

func (s *slowSource) Name() string { return "slow-source" }
func (s *slowSource) Search(ctx context.Context, _ discovery.DiscoveryQuery) ([]model.MCPServerRef, error) {
	select {
	case <-time.After(s.delay):
		return nil, errors.New("should have been cancelled")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (s *slowSource) HealthCheck(_ context.Context) error { return nil }

// --- RecommendationSet tests ---

func TestRecommendationSetCategorization(t *testing.T) {
	scorer := discovery.NewQualityScorer()
	input := []model.MCPServerRef{
		{Name: "must-have", QualityScore: 95, RiskLevel: "low"},
		{Name: "nice", QualityScore: 75, RiskLevel: "low"},
		{Name: "risky", QualityScore: 60, RiskLevel: "high"},
	}

	recs := scorer.Categorize(input)
	if len(recs.MustHave) != 1 || recs.MustHave[0] != "must-have" {
		t.Errorf("MustHave = %v, want [must-have]", recs.MustHave)
	}
	if len(recs.NiceToHave) != 1 || recs.NiceToHave[0] != "nice" {
		t.Errorf("NiceToHave = %v, want [nice]", recs.NiceToHave)
	}
	if len(recs.Avoid) != 1 || recs.Avoid[0] != "risky" {
		t.Errorf("Avoid = %v, want [risky]", recs.Avoid)
	}
}

// --- DiscoveryResult tests ---

func TestDiscoveryResultScoredAt(t *testing.T) {
	src := &mockSource{
		name: "test",
		results: []model.MCPServerRef{
			{Name: "tool-1", QualityScore: 80},
		},
	}

	engine := discovery.NewEngine([]discovery.SourceReader{src})
	result, err := engine.Search(context.Background(), discovery.DiscoveryQuery{
		Role: model.RoleDataScience,
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if result.ScoredAt == "" {
		t.Error("ScoredAt should be set after Search")
	}
}

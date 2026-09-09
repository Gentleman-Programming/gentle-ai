package model_test

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func TestPiSubscriptionsOrder(t *testing.T) {
	subs := model.PiSubscriptionsOrder()
	if len(subs) != 4 {
		t.Fatalf("expected 4 subscriptions, got %d", len(subs))
	}
	for _, sub := range subs {
		desc := model.PiSubscriptionDescription(sub)
		if desc == "" {
			t.Errorf("empty description for subscription %s", sub)
		}
	}
}

func TestPiPresetsCompleteness(t *testing.T) {
	agents := model.PiConfigurableAgents()
	subs := model.PiSubscriptionsOrder()

	validThinking := map[string]bool{
		"":        true,
		"off":     true,
		"minimal": true,
		"low":     true,
		"medium":  true,
		"high":    true,
		"xhigh":   true,
		"max":     true,
	}

	for _, sub := range subs {
		preset := model.PiPresetForSubscription(sub)
		if len(preset) == 0 {
			t.Fatalf("preset for %s returned empty mapping", sub)
		}

		for _, agent := range agents {
			entry, ok := preset[agent]
			if !ok {
				t.Errorf("subscription %s missing agent %s", sub, agent)
				continue
			}
			if entry.Model == "" {
				t.Errorf("subscription %s agent %s has empty model", sub, agent)
			}
			if !validThinking[entry.Thinking] {
				t.Errorf("subscription %s agent %s has invalid thinking: %q", sub, agent, entry.Thinking)
			}
		}
	}
}

func TestPiPresetDefaultFallback(t *testing.T) {
	preset := model.PiPresetForSubscription("unknown")
	claude := model.PiPresetClaude()

	if len(preset) != len(claude) {
		t.Fatalf("expected default fallback to Claude, got len %d vs %d", len(preset), len(claude))
	}
}

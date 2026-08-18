package service

import (
	"testing"
	"time"

	"powerlevel/internal/providers/cardcatalog"
)

func TestAnalysisCacheEvictsLeastRecentlyUsed(t *testing.T) {
	t.Parallel()
	cache := newAnalysisCache(2)
	now := time.Now()
	cache.set("a", testAnalysis("a"), now.Add(time.Minute))
	cache.set("b", testAnalysis("b"), now.Add(time.Minute))
	if _, ok := cache.get("a", now); !ok {
		t.Fatal("expected a cache hit")
	}
	cache.set("c", testAnalysis("c"), now.Add(time.Minute))
	if _, ok := cache.get("b", now); ok {
		t.Fatal("expected least recently used entry b to be evicted")
	}
	if _, ok := cache.get("a", now); !ok {
		t.Fatal("expected recently used entry a to remain")
	}
}

func TestAnalysisCacheExpiresAndClones(t *testing.T) {
	t.Parallel()
	cache := newAnalysisCache(2)
	now := time.Now()
	analysis := testAnalysis("a")
	cache.set("a", analysis, now.Add(time.Second))
	analysis.Results["provider"] = ProviderResult{Status: "changed"}

	cached, ok := cache.get("a", now)
	if !ok || cached.Results["provider"].Status != "success" {
		t.Fatalf("cache was mutated through caller-owned map: %+v", cached)
	}
	cached.Results["provider"] = ProviderResult{Status: "changed again"}
	second, _ := cache.get("a", now)
	if second.Results["provider"].Status != "success" {
		t.Fatal("cache hit returned shared mutable state")
	}
	cachedMetrics := cached.Results["nested"].Metrics["suggestions"].(map[string]any)
	cachedItems := cachedMetrics["items"].([]any)
	cachedItems[0].(map[string]any)["label"] = "changed"
	third, _ := cache.get("a", now)
	thirdItems := third.Results["nested"].Metrics["suggestions"].(map[string]any)["items"].([]any)
	if thirdItems[0].(map[string]any)["label"] != "original" {
		t.Fatal("nested metrics were shared across cache hits")
	}
	cached.Recommendations[0].Cards[0].Fills[0].Label = "changed"
	fourth, _ := cache.get("a", now)
	if fourth.Recommendations[0].Cards[0].Fills[0].Label != "加速" {
		t.Fatal("recommendation fills were shared across cache hits")
	}
	if _, ok := cache.get("a", now.Add(time.Second)); ok {
		t.Fatal("expected expired entry to be removed")
	}
}

func testAnalysis(id string) Analysis {
	return Analysis{
		Status:          "success",
		Deck:            DeckSummary{ID: id, Commanders: []string{"Commander"}},
		Recommendations: []RecommendationGroup{{Header: "Mana", Cards: []RecommendedCard{{Card: cardcatalog.Card{Name: "Rock"}, Fills: []RecommendationFill{{ID: "ramp", Label: "加速", Gap: 2}}}}}},
		Results: map[string]ProviderResult{
			"provider": {Status: "success", Metrics: map[string]any{"score": 5.0}},
			"nested": {
				Status: "success",
				Metrics: map[string]any{
					"suggestions": map[string]any{"items": []any{map[string]any{"label": "original"}}},
				},
			},
		},
	}
}

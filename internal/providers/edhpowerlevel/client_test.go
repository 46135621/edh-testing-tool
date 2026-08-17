package edhpowerlevel

import "testing"

func TestNormalizeBracketDetails(t *testing.T) {
	t.Parallel()
	details := normalizeBracketDetails(bracketDetails{
		RulesBracketReasons:    []string{" No Game Changers ", "No Game Changers", "", "Only Late Game 2-Card Combos"},
		EvaluatedBracketReason: "  Recommended bracket is 4 based on power level.  ",
	})
	if len(details.RulesBracketReasons) != 2 {
		t.Fatalf("unexpected reasons: %#v", details.RulesBracketReasons)
	}
	if details.RulesBracketReasons[0] != "No Game Changers" || details.EvaluatedBracketReason != "Recommended bracket is 4 based on power level." {
		t.Fatalf("unexpected details: %+v", details)
	}
}

func TestNormalizeMetrics(t *testing.T) {
	t.Parallel()
	metrics := normalizeMetrics(map[string]string{
		"Power Level":         "5.55 / 10",
		"Efficiency":          "4.82 / 10",
		"Score":               "447 / 1000",
		"Average Playability": "51.8%",
		"Rules Bracket":       "2",
		"Evaluated Bracket":   "Commander Bracket: 4",
	})
	for key, want := range map[string]float64{
		"power_level": 5.55, "efficiency": 4.82, "score": 447, "average_playability": 51.8,
		"rules_bracket": 2, "evaluated_bracket": 4,
	} {
		if got := metrics[key]; got != want {
			t.Fatalf("%s = %#v, want %v", key, got, want)
		}
	}
}

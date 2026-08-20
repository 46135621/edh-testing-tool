package edhpowerlevel

import (
	"testing"

	"powerlevel/internal/providers/spellbook"
)

func TestCurve(t *testing.T) {
	// de(value, stops, offset): offset=1 gives a 0..len-1 ramp scaled by offset.
	if got := curve(0, powerCurve, 1); got != 0 {
		t.Fatalf("curve(0) = %v, want 0", got)
	}
	if got := curve(500, []float64{0, 250, 320, 350, 380, 420, 470, 560, 760, 890, 1000}, 1); got <= 0 {
		t.Fatalf("curve(500) = %v, want > 0", got)
	}
}

func TestHypergeometricCDF(t *testing.T) {
	// Deck of 99 with 37 lands, drawing 7: P(0 lands) via hypergeometric CDF.
	cdf := hypergeometricCDF(0, 99, 37, 7)
	if cdf < 0 || cdf > 1 {
		t.Fatalf("CDF out of range: %v", cdf)
	}
	// Drawing 0 lands but asking x to exceed possible draws must clamp to a valid range.
	if got := hypergeometricCDF(100, 99, 37, 7); got > 1 {
		t.Fatalf("CDF overshoot not clamped: %v", got)
	}
	// No successes to draw => probability of drawing <= x lands is 1 for x >= 0 draws of none.
	if got := hypergeometricCDF(0, 99, 0, 7); got != 0 {
		t.Fatalf("CDF with K=0 = %v, want 0 via early return", got)
	}
}

func TestAveragePlayabilityNonZero(t *testing.T) {
	// A single non-land card with a modest mana cost over a normal deck should yield a
	// playability strictly between 0 and 1, confirming the pips/lands probabilities run.
	scored := []*scoredCard{
		{card: &card{name: "Cultivate"}, quantity: 1, isLand: false, cmc: 3, pips: map[string]int{"G": 1}},
	}
	producers := map[string]int{"R": 0, "W": 0, "G": 8, "U": 0, "B": 0, "C": 0}
	got := averagePlayability(scored, producers, 37, 62, 1)
	if got <= 0 || got >= 1 {
		t.Fatalf("averagePlayability = %v, want in (0,1)", got)
	}
}

func TestComputeBracketComboClassification(t *testing.T) {
	cmc := map[string]int{"dualcaster mage": 3, "twinflame": 2, "walking ballista": 0, "heliod, sun-crowned": 3}

	// ManaValueNeeded (2) + battlefield cmcs (3+2) = 7 => early combo. ProducesFeatureIDs
	// uses 9999 (not in gameDefiningProducers) so the combo is classified as a real combo.
	early := []spellbook.Combo{
		{
			Name:               "Dualcaster Mage + Twinflame",
			ManaValueNeeded:    2,
			ProducesFeatureIDs: []int{9999},
			Components: []spellbook.Component{
				{Name: "Dualcaster Mage", Zone: "B,Hand"},
				{Name: "Twinflame", Zone: "B,Hand"},
			},
		},
	}
	_, details := computeBracket(nil, 0, early, cmc)
	if len(details.EarlyTwoCardComboNames) != 1 || details.EarlyTwoCardCombos != 1 {
		t.Fatalf("early combo not classified: %+v", details.EarlyTwoCardComboNames)
	}
	if len(details.lateComboNames) != 0 {
		t.Fatalf("unexpected late combos: %v", details.lateComboNames)
	}

	// Higher mana value pushes the same combo past 7 => late combo.
	late := []spellbook.Combo{
		{
			Name:               "Dualcaster Mage + Twinflame",
			ManaValueNeeded:    5,
			ProducesFeatureIDs: []int{9999},
			Components: []spellbook.Component{
				{Name: "Dualcaster Mage", Zone: "B,Hand"},
				{Name: "Twinflame", Zone: "B,Hand"},
			},
		},
	}
	_, details2 := computeBracket(nil, 0, late, cmc)
	if len(details2.EarlyTwoCardComboNames) != 0 {
		t.Fatalf("late combo misclassified as early: %+v", details2.EarlyTwoCardComboNames)
	}
	if len(details2.lateComboNames) != 1 {
		t.Fatalf("late combo not recorded: %v", details2.lateComboNames)
	}

	// An early 2-card combo forces a minimum "rules bracket" of 4: the early-combo
	// restriction first bites at Bracket 4 (only Brackets 4-5 may run one), which
	// carries through to the recommended/evaluated bracket.
	rules, details3 := computeBracket(nil, 0, early, cmc)
	if rules != 4 {
		t.Fatalf("rules bracket with early combo = %d, want 4", rules)
	}
	if len(details3.EarlyTwoCardComboNames) != 1 {
		t.Fatalf("early combo names not preserved: %+v", details3.EarlyTwoCardComboNames)
	}
}

func TestLookupCardFlavorName(t *testing.T) {
	cards := map[string]*card{
		"cyclonic rift":     {name: "Cyclonic Rift", flavorNames: []string{"Hope's Aero Magic"}},
		"hope's aero magic": {name: "Cyclonic Rift", flavorNames: []string{"Hope's Aero Magic"}},
	}
	if got := lookupCard(cards, "Hope's Aero Magic"); got == nil || got.name != "Cyclonic Rift" {
		t.Fatalf("flavor-name lookup failed: %+v", got)
	}
	if got := lookupCard(cards, "Cyclonic Rift"); got == nil || got.name != "Cyclonic Rift" {
		t.Fatalf("canonical lookup failed: %+v", got)
	}
	// Split-card front face fallback.
	split := map[string]*card{
		"boggart trawler": {name: "Boggart Trawler // Boggart Bog"},
	}
	if got := lookupCard(split, "Boggart Trawler // Boggart Bog"); got == nil {
		t.Fatalf("split front-face lookup failed: %+v", got)
	}
}

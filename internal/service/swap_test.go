package service

import (
	"context"
	"strings"
	"testing"

	"powerlevel/internal/deck"
	"powerlevel/internal/providers/cardcatalog"
)

type swapCatalog struct{ cards map[string]cardcatalog.Card }

func (s swapCatalog) Lookup(_ context.Context, names []string) (map[string]cardcatalog.Card, error) {
	result := make(map[string]cardcatalog.Card)
	for _, name := range names {
		for key, card := range s.cards {
			if strings.EqualFold(key, name) {
				result[strings.ToLower(name)] = card
				break
			}
		}
	}
	return result, nil
}

func (s swapCatalog) Search(_ context.Context, _ string, _ int) ([]cardcatalog.Card, error) {
	return nil, nil
}

func TestCompareSwapChangesConstructionMetricsAndPreservesInput(t *testing.T) {
	legal := map[string]string{"commander": "legal"}
	catalog := swapCatalog{cards: map[string]cardcatalog.Card{
		"Commander": {Name: "Commander", TypeLine: "Creature", ColorIdentity: []string{"G"}, OracleText: "Create a token.", Legalities: legal},
		"Plains":    {Name: "Plains", TypeLine: "Basic Land — Plains", OracleText: "{T}: Add {W}.", Legalities: legal},
		"Shock":     {Name: "Shock", TypeLine: "Instant", OracleText: "Exile target creature.", ColorIdentity: []string{"G"}, Legalities: legal},
		"Sol Ring":  {Name: "Sol Ring", TypeLine: "Artifact", OracleText: "{T}: Add {C}{C}.", Legalities: legal},
	}}
	a := &Analyzer{cards: catalog}
	input := deck.Deck{Commanders: []deck.Card{{Name: "Commander", Quantity: 1}}, Mainboard: []deck.Card{{Name: "Sol Ring", Quantity: 1}, {Name: "Plains", Quantity: 98}}}
	comparison, err := a.CompareSwap(context.Background(), input.ExportPlainText(), "Sol Ring", "Shock")
	if err != nil {
		t.Fatalf("CompareSwap failed: %v", err)
	}
	if comparison.Before.CardCount != 100 || comparison.After.CardCount != 100 {
		t.Fatalf("card count changed: %+v", comparison)
	}
	if comparison.Added.Name != "Shock" || !comparison.Legality.Valid {
		t.Fatalf("unexpected swap result: %+v", comparison)
	}
	if input.Mainboard[0].Name != "Sol Ring" || input.Mainboard[0].Quantity != 1 {
		t.Fatal("CompareSwap mutated input deck")
	}
	for _, delta := range comparison.Deltas {
		if delta.ID == "ramp" && delta.Delta != -1 {
			t.Fatalf("ramp delta = %d, want -1", delta.Delta)
		}
		if delta.ID == "single_interaction" && delta.Delta != 1 {
			t.Fatalf("interaction delta = %d, want 1", delta.Delta)
		}
	}
	if _, err := deck.ParsePlainText(comparison.UpdatedDecklist); err != nil {
		t.Fatalf("updated decklist is not parseable: %v", err)
	}
}

func TestCompareSwapRejectsCommanderAndOffColor(t *testing.T) {
	legal := map[string]string{"commander": "legal"}
	a := &Analyzer{cards: swapCatalog{cards: map[string]cardcatalog.Card{
		"Commander": {Name: "Commander", TypeLine: "Creature", ColorIdentity: []string{"G"}, Legalities: legal},
		"Plains":    {Name: "Plains", TypeLine: "Basic Land", OracleText: "{T}: Add {W}.", Legalities: legal},
		"Island":    {Name: "Island", TypeLine: "Basic Land", OracleText: "{T}: Add {U}.", ColorIdentity: []string{"U"}, Legalities: legal},
	}}}
	list := "Commander\n1 Commander\n\nDeck\n1 Plains\n98 Island"
	if _, err := a.CompareSwap(context.Background(), list, "Commander", "Island"); err != ErrCommanderSwap {
		t.Fatalf("commander removal error = %v", err)
	}
	if _, err := a.CompareSwap(context.Background(), list, "Plains", "Island"); err != ErrColorIdentity {
		t.Fatalf("off-color error = %v", err)
	}
}

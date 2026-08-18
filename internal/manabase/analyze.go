package manabase

import (
	"sort"

	"powerlevel/internal/providers/cardcatalog"
)

// Analyze runs the stage-1 mana-base analysis over a deck's Scryfall card payloads.
// It is the public entry point: classify cards into a ManabaseDeck, compute the
// Karsten land target, then the per-color source requirement, and assemble a Report.
func Analyze(entries []ClassifyEntry) Report {
	deck := classify(entries)

	// Land count = weighted land sources (real land slots).
	actualLands := 0
	for _, s := range deck.Sources {
		if s.IsLand {
			actualLands += int(s.Weight)
		}
	}

	deckSize := deck.TotalCards - deck.CommanderCount
	targetLands := singletonLandTarget(
		deck.TotalCards,
		deck.CommanderCount,
		deck.AverageManaValue,
		float64(deck.RampAndDrawUnderThree),
		float64(deck.FastMana),
	)

	report := Report{
		ActualLands:           actualLands,
		TargetLands:           targetLands,
		LandDelta:             float64(actualLands) - targetLands,
		AverageManaValue:      deck.AverageManaValue,
		RampAndDrawUnderThree: deck.RampAndDrawUnderThree,
		FastMana:              deck.FastMana,
		ColorFindings:         buildColorFindings(deck, deckSize),
	}
	return report
}

// buildColorFindings computes a per-color source requirement for each color that the
// deck demands. For each color it totals the effective (weighted) sources producing
// that color, then finds the most demanding spell of that color and the number of
// sources Karsten's threshold requires.
func buildColorFindings(deck ManabaseDeck, deckSize int) []ColorFinding {
	// Effective weighted sources per color.
	actual := map[ManaColor]float64{}
	totalLands := 0
	for _, s := range deck.Sources {
		if s.IsLand {
			totalLands++
		}
		for _, c := range s.Produces {
			if c != ColorColorless {
				actual[c] += s.Weight
			}
		}
	}

	// The most demanding spell of each color (highest pip requirement, breaking ties
	// by higher mana value) drives the source requirement.
	type demand struct {
		pips  int
		mv    int
		spell string
	}
	demanding := map[ManaColor]demand{}
	for _, sp := range deck.Spells {
		for color, pips := range sp.Pips {
			if color == ColorColorless || pips <= 0 {
				continue
			}
			prev, ok := demanding[color]
			if !ok || pips > prev.pips || (pips == prev.pips && sp.ManaValue > prev.mv) {
				demanding[color] = demand{pips: pips, mv: sp.ManaValue, spell: sp.Name}
			}
		}
	}

	colors := make([]ManaColor, 0, len(demanding))
	for color := range demanding {
		colors = append(colors, color)
	}
	sort.Slice(colors, func(i, j int) bool { return colors[i] < colors[j] })

	findings := make([]ColorFinding, 0, len(colors))
	for _, color := range colors {
		d := demanding[color]
		required := sourcesNeeded(deckSize, totalLands, d.pips, d.mv, true)
		findings = append(findings, ColorFinding{
			Color:           color,
			ActualSources:   round2(actual[color]),
			RequiredSources: required,
			DrivingSpell:    d.spell,
		})
	}
	return findings
}

// Entry adapts a resolved Scryfall card plus its deck quantity into a ClassifyEntry.
// It exists so callers who already hold a cardcatalog.Card and quantity needn't
// construct the struct by hand.
func Entry(card cardcatalog.Card, quantity int, commander bool) ClassifyEntry {
	return ClassifyEntry{Card: card, Quantity: quantity, IsCommander: commander}
}

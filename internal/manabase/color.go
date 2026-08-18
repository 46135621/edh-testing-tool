package manabase

import (
	"encoding/json"
	"strings"
)

// ManaColor identifies one of the five colors, plus a Colorless bucket that also
// absorbs snow ({S}) pips, mirroring DeckFlow's ManaColor mapping.
type ManaColor uint8

const (
	ColorWhite ManaColor = iota
	ColorBlue
	ColorBlack
	ColorRed
	ColorGreen
	ColorColorless
)

// String returns the single-letter shorthand (W/U/B/R/G/C) used for display and
// debugging. It matches Scryfall's produced_mana letters for the first five.
func (c ManaColor) String() string {
	switch c {
	case ColorWhite:
		return "W"
	case ColorBlue:
		return "U"
	case ColorBlack:
		return "B"
	case ColorRed:
		return "R"
	case ColorGreen:
		return "G"
	case ColorColorless:
		return "C"
	default:
		return "?"
	}
}

// MarshalJSON encodes a ManaColor as its single-letter shorthand so the API emits
// "W"/"U"/"B"/"R"/"G"/"C" rather than the underlying integer.
func (c ManaColor) MarshalJSON() ([]byte, error) {
	return []byte(`"` + c.String() + `"`), nil
}

// UnmarshalJSON decodes a single-letter shorthand ("W"/"U"/"B"/"R"/"G"/"C") back
// into a ManaColor, the inverse of MarshalJSON. It also tolerates a bare number
// (a previously-serialized integer) so older cache entries still round-trip.
func (c *ManaColor) UnmarshalJSON(data []byte) error {
	if len(data) >= 2 && data[0] == '"' {
		var token string
		if err := json.Unmarshal(data, &token); err != nil {
			return err
		}
		if color, ok := mapSymbol(strings.ToUpper(token)); ok {
			*c = color
			return nil
		}
		return nil
	}
	var number uint8
	if err := json.Unmarshal(data, &number); err == nil {
		*c = ManaColor(number)
	}
	return nil
}

// mapSymbol maps a single Scryfall color/colorless letter to a ManaColor. Snow
// ({S}) folds into the Colorless bucket exactly as DeckFlow's ManaCostParser does.
// It returns false for anything unrecognized.
func mapSymbol(token string) (ManaColor, bool) {
	switch token {
	case "W":
		return ColorWhite, true
	case "U":
		return ColorBlue, true
	case "B":
		return ColorBlack, true
	case "R":
		return ColorRed, true
	case "G":
		return ColorGreen, true
	case "C", "S":
		return ColorColorless, true
	default:
		return 0, false
	}
}

// mapColors converts Scryfall produced_mana letters into de-duplicated ManaColors.
func mapColors(produced []string) []ManaColor {
	var colors []ManaColor
	for _, letter := range produced {
		if c, ok := mapSymbol(strings.ToUpper(strings.TrimSpace(letter))); ok {
			if !containsColor(colors, c) {
				colors = append(colors, c)
			}
		}
	}
	return colors
}

func containsColor(colors []ManaColor, c ManaColor) bool {
	for _, existing := range colors {
		if existing == c {
			return true
		}
	}
	return false
}

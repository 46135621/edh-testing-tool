package manabase

// ManaSource is a land or partial mana source and the colors it can produce. Weight
// allows discounting fragile or conditional sources per Karsten's counting rules
// (mana dork ≈ 0.5, Signet ≈ 0.75). Ported from DeckFlow.Core.Manabase.ManaSource.
type ManaSource struct {
	Name string

	// Produces is the set of colors this source can tap for.
	Produces []ManaColor

	// Weight is the effective source weight (1.0 for a normal land).
	Weight float64

	// IsLand is true when this source occupies a land slot (counts toward the
	// land-drop total), even when its color weight is discounted.
	IsLand bool

	// EntersUntapped is true when it can produce mana the turn it is played.
	EntersUntapped bool

	// ManaAmount is how much mana this source makes per activation (Sol Ring = 2).
	ManaAmount int

	// IsCommander is true for a source contributed by a command-zone card; such a
	// source is not drawn into the simulated library but still counts toward color
	// supply. The simplified stage-1 classifier does not populate this.
	IsCommander bool
}

// SpellRequirement is a colored spell whose castability we want to check. It is a
// trimmed port of DeckFlow.Core.Manabase.SpellRequirement.
type SpellRequirement struct {
	Name string

	// ManaValue is the total mana value — the turn the spell is cast on curve.
	ManaValue int

	// Pips holds colored pip counts by color (colors with zero pips are omitted).
	Pips map[ManaColor]int

	// IsGold is true when the card needs more than one color.
	IsGold bool

	// IsManaSource is true when the card is itself a mana rock/dork. Such cards are
	// excluded from castability rows but still feed the source pools.
	IsManaSource bool

	// IsCommander marks the deck's commander (pinned to the top of any listing).
	IsCommander bool
}

// ManabaseDeck is a fully classified deck ready for mana-base analysis: its lands,
// its colored spells, and the aggregate numbers the land-count formula needs.
// Ported from DeckFlow.Core.Manabase.ManasourceDeck, trimmed to stage-1 fields.
type ManabaseDeck struct {
	// TotalCards is the total cards including commanders (typically 100).
	TotalCards int

	// CommanderCount is commanders in the command zone.
	CommanderCount int

	// Sources is all lands / mana sources in the deck.
	Sources []ManaSource

	// Spells is colored spells whose castability we want to check.
	Spells []SpellRequirement

	// AverageManaValue is the mean mana value of the non-land cards.
	AverageManaValue float64

	// RampAndDrawUnderThree is the count of ramp/card-draw spells of mana value 2
	// or less (the −0.28 land-target credit input).
	RampAndDrawUnderThree int

	// FastMana is the count of 0-cost mana artifacts (Lotus, Moxen).
	FastMana int

	// IsSingleton is true for a Commander deck (uses the 99-card formula).
	IsSingleton bool
}

// ColorFinding reports one color's source supply versus its toughest requirement in
// the deck. Ported from DeckFlow.Core.Manabase.ColorSourceFinding, trimmed.
type ColorFinding struct {
	// Color is the color examined.
	Color ManaColor `json:"color"`

	// ActualSources is effective sources of this color currently in the deck (weighted).
	ActualSources float64 `json:"actual_sources"`

	// RequiredSources is sources required by the most demanding spell of this color.
	RequiredSources int `json:"required_sources"`

	// DrivingSpell is the spell that drove the requirement (the worst single-spell deficit).
	DrivingSpell string `json:"driving_spell"`
}

// Report is the trimmed mana-base report: land count, ramp, per-color sources, and
// a verdict. Ported from DeckFlow.Core.Manabase.ManasourceReport.
type Report struct {
	// ActualLands is lands actually in the deck.
	ActualLands int `json:"actual_lands"`

	// TargetLands is the Karsten-recommended land count for the curve.
	TargetLands float64 `json:"target_lands"`

	// LandDelta is actual minus target; negative means too few lands.
	LandDelta float64 `json:"land_delta"`

	// AverageManaValue is the mean non-land mana value the regression used.
	AverageManaValue float64 `json:"average_mana_value"`

	// RampAndDrawUnderThree is the ramp/draw credit input.
	RampAndDrawUnderThree int `json:"ramp_and_draw_under_three"`

	// FastMana is the 0-cost fast-mana credit input.
	FastMana int `json:"fast_mana"`

	// ColorFindings is per-color source findings. Stage 1 keeps deck order; no
	// composite tail-risk ordering is applied.
	ColorFindings []ColorFinding `json:"color_findings"`
}

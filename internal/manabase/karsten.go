package manabase

import "math"

// Frank Karsten's mana-base math: the land-count-vs-curve regression and the
// colored-source requirement for a given pip pattern, reproduced from "How Many
// Sources Do You Need to Consistently Cast Your Spells? A 2022 Update".
//
// This is a port of DeckFlow.Core.Manabase.KarstenManabase, trimmed to the two
// functional pieces this project needs: the singleton land-target regression and
// the conditional-hypergeometric source requirement. The cEDH recalibration path
// is intentionally omitted (it depends on an external commander baseline we do
// not have).
const (
	// landIntercept and landMvSlope are Karsten's published 60-card land-count
	// regression (interior = intercept + slope·MV). The singleton target scales the
	// interior by deck size; keep them as named constants so the 60-card and
	// singleton targets can never silently diverge again.
	landIntercept = 19.59
	landMvSlope   = 1.90

	// rampDrawCredit is the land-target credit per ramp/card-draw spell of MV <= 2.
	rampDrawCredit = 0.28
)

// singletonLandTarget returns Karsten's recommended land count for a singleton
// (Commander) deck.
//
//	totalCards          deck size including commanders (typically 100)
//	commanderCount      commanders in the command zone (1, or 2 for partners)
//	averageManaValue    mean mana value of non-land cards
//	rampAndDrawUnder3   count of ramp/card-draw spells of mana value 2 or less
//	fastMana            count of 0-cost mana artifacts (Lotus, Moxen); Sol Ring ≈ 0.8
func singletonLandTarget(totalCards, commanderCount int, averageManaValue, rampAndDrawUnder3, fastMana float64) float64 {
	scale := float64(totalCards-commanderCount) / 60.0
	interior := landIntercept + (landMvSlope * averageManaValue) + (0.27 * float64(commanderCount))
	return (scale * interior) - (rampDrawCredit * rampAndDrawUnder3) - fastMana - 1.35
}

// consistencyThreshold returns Karsten's consistency target for a spell of the given
// mana value: (89 + M)% — 90% for one-drops rising to 96% for seven-drops.
func consistencyThreshold(manaValue int) float64 {
	pct := 89 + manaValue
	if pct < 90 {
		pct = 90
	}
	return clamp01(float64(pct) / 100.0)
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 0.99 {
		return 0.99
	}
	return x
}

// cardsSeenByTurn returns cards seen by the given turn assuming a 7-card opener.
// On the play you skip the first draw step; on the draw you do not.
func cardsSeenByTurn(turn int, onPlay bool) int {
	if onPlay {
		return 7 + (turn - 1)
	}
	return 7 + turn
}

// castConsistency returns the conditional probability of having at least `pips`
// sources of a color by the spell's on-curve turn, given at least `manaValue` lands
// were drawn — exactly the metric Karsten's tables report. A pips value <= 0 returns
// 1.0: with no colored requirement, the (already-given) land condition is trivially
// satisfied.
//
//	deckSize      cards in the library (exclude commanders in the command zone)
//	totalLands    total lands in the library
//	colorSources  lands (or partial sources) producing the color in question
//	pips          colored pips of that color in the cost
//	manaValue     the spell's mana value — also the turn it is cast on curve
func castConsistency(deckSize, totalLands, colorSources, pips, manaValue int, onPlay bool) float64 {
	if pips <= 0 {
		return 1.0
	}

	draws := cardsSeenByTurn(manaValue, onPlay)
	otherLands := totalLands - colorSources
	nonland := deckSize - totalLands

	pLandsEnough := atLeast(deckSize, totalLands, draws, manaValue)
	if pLandsEnough <= 0.0 {
		return 0.0
	}

	// P(sources >= pips AND lands >= M): triple-category (sources, other lands, nonland).
	logDenomDraw := logChoose(deckSize, draws)
	joint := 0.0
	maxS := colorSources
	if draws < maxS {
		maxS = draws
	}
	for s := pips; s <= maxS; s++ {
		maxO := otherLands
		if draws-s < maxO {
			maxO = draws - s
		}
		for o := 0; o <= maxO; o++ {
			if s+o < manaValue {
				continue
			}
			rest := draws - s - o
			if rest < 0 || rest > nonland {
				continue
			}
			logTerm := logChoose(colorSources, s) +
				logChoose(otherLands, o) +
				logChoose(nonland, rest) -
				logDenomDraw
			joint += math.Exp(logTerm)
		}
	}
	return clamp1(joint / pLandsEnough)
}

// sourcesNeeded returns the minimum colored sources required to cast a `pips`-pip
// spell of the given mana value on curve at Karsten's (89 + M)% threshold. It
// returns totalLands if even an all-on-color base falls short.
func sourcesNeeded(deckSize, totalLands, pips, manaValue int, onPlay bool) int {
	threshold := consistencyThreshold(manaValue)
	for sources := pips; sources <= totalLands; sources++ {
		if castConsistency(deckSize, totalLands, sources, pips, manaValue, onPlay) >= threshold {
			return sources
		}
	}
	return totalLands
}

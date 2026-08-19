package service

import (
	"powerlevel/internal/providers/cardcatalog"
)

// gameChangerNames is the Commander format's official Game Changers list (the 53 cards
// that push a deck toward Bracket 3+). Names come from the `is:gamechanger` tag in
// Scryfall, which mirrors the Wizards of the Coast Commander Brackets announcement.
// Names are normalized via normalizeCardName so a split/multiface card matches its
// front face too.
var gameChangerNames = func() map[string]struct{} {
	names := []string{
		"Ad Nauseam",
		"Ancient Tomb",
		"Aura Shards",
		"Biorhythm",
		"Bolas's Citadel",
		"Braids, Cabal Minion",
		"Chrome Mox",
		"Coalition Victory",
		"Consecrated Sphinx",
		"Crop Rotation",
		"Cyclonic Rift",
		"Demonic Tutor",
		"Drannith Magistrate",
		"Enlightened Tutor",
		"Farewell",
		"Field of the Dead",
		"Fierce Guardianship",
		"Force of Will",
		"Gaea's Cradle",
		"Gamble",
		"Gifts Ungiven",
		"Glacial Chasm",
		"Grand Arbiter Augustin IV",
		"Grim Monolith",
		"Humility",
		"Imperial Seal",
		"Intuition",
		"Jeska's Will",
		"Lion's Eye Diamond",
		"Mana Vault",
		"Mishra's Workshop",
		"Mox Diamond",
		"Mystical Tutor",
		"Narset, Parter of Veils",
		"Natural Order",
		"Necropotence",
		"Notion Thief",
		"Opposition Agent",
		"Orcish Bowmasters",
		"Panoptic Mirror",
		"Rhystic Study",
		"Seedborn Muse",
		"Serra's Sanctum",
		"Smothering Tithe",
		"Survival of the Fittest",
		"Teferi's Protection",
		"Tergrid, God of Fright",
		"Thassa's Oracle",
		"The One Ring",
		"The Tabernacle at Pendrell Vale",
		"Underworld Breach",
		"Vampiric Tutor",
		"Worldly Tutor",
	}
	set := make(map[string]struct{}, len(names))
	for _, name := range names {
		set[normalizeCardName(name)] = struct{}{}
	}
	return set
}()

// isGameChanger reports whether a card is on the Commander Game Changers list. It
// matches by normalized name against both the card name and any faces (so a split or
// double-faced Game Changer such as Tergrid still matches on its front face).
func isGameChanger(card cardcatalog.Card) bool {
	if _, ok := gameChangerNames[normalizeCardName(card.Name)]; ok {
		return true
	}
	for _, face := range card.Faces {
		if _, ok := gameChangerNames[normalizeCardName(face.Name)]; ok {
			return true
		}
	}
	return false
}

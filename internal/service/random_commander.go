package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"powerlevel/internal/providers/cardcatalog"
	"powerlevel/internal/providers/edhrec"
)

// ErrCommanderNotLegal is returned when a requested commander cannot be run as a
// Commander according to its Scryfall legality.
var ErrCommanderNotLegal = errors.New("card is not legal as a Commander")

// ErrCommanderPairInvalid is returned when a second commander is supplied but the
// pair does not share a legal partner relationship (partner / partner with /
// friends forever / background).
var ErrCommanderPairInvalid = errors.New("these commanders cannot be paired")

// ErrRandomCommanderUnavailable is returned when the popularity list cannot be
// loaded and no random commander can be drawn.
var ErrRandomCommanderUnavailable = errors.New("random commander list is unavailable")

// commanderRankingsCache memoizes the EDHREC commander popularity list in-process so
// repeated random draws (or several users) don't re-fetch it every time.
type commanderRankingsCache struct {
	mu        sync.Mutex
	rankings  []edhrec.CommanderRanking
	expiresAt time.Time
	ttl       time.Duration
}

func newCommanderRankingsCache(ttl time.Duration) *commanderRankingsCache {
	if ttl <= 0 {
		ttl = 6 * time.Hour
	}
	return &commanderRankingsCache{ttl: ttl}
}

func (c *commanderRankingsCache) get(load func(context.Context) ([]edhrec.CommanderRanking, error), ctx context.Context) ([]edhrec.CommanderRanking, error) {
	c.mu.Lock()
	if c.rankings != nil && time.Now().Before(c.expiresAt) {
		r := c.rankings
		c.mu.Unlock()
		return r, nil
	}
	c.mu.Unlock()

	rankings, err := load(ctx)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.rankings = rankings
	c.expiresAt = time.Now().Add(c.ttl)
	c.mu.Unlock()
	return rankings, nil
}

// ResolvedCommander is a commander name plus its full Scryfall card, so the random
// draw and pairing logic can return display-ready payloads (name, art, legality).
type ResolvedCommander struct {
	Name          string           `json:"name"`
	Card          cardcatalog.Card `json:"card"`
	ColorIdentity []string         `json:"color_identity"`
	IsPartner     bool             `json:"is_partner"`
}

// RandomCommander draws one commander from the EDHREC popularity list, weighted so
// that less-popular commanders are more likely, then resolves it to full Scryfall
// card data. A draw that resolves to a non-legal or non-legendary creature is retried
// a bounded number of times before reporting the list as unusable.
func (a *Analyzer) RandomCommander(ctx context.Context) (ResolvedCommander, error) {
	if a.edhrec == nil || a.cards == nil {
		return ResolvedCommander{}, ErrCardData
	}
	rankings, err := a.rankingsCache.get(a.edhrec.CommanderRankings, ctx)
	if err != nil {
		return ResolvedCommander{}, ErrRandomCommanderUnavailable
	}

	const maxAttempts = 8
	for attempt := 0; attempt < maxAttempts; attempt++ {
		name := a.weightedCommanderName(rankings)
		card, err := a.LookupCard(ctx, name)
		if err != nil {
			continue
		}
		if !hasUsableCardData(card) {
			continue
		}
		if card.Legalities["commander"] != "legal" {
			continue
		}
		if !isLegendaryCreature(card) {
			continue
		}
		return ResolvedCommander{
			Name:          card.Name,
			Card:          card,
			ColorIdentity: sortedColors(card.ColorIdentity),
			IsPartner:     hasPartnerKeyword(card),
		}, nil
	}
	return ResolvedCommander{}, ErrRandomCommanderUnavailable
}

// ResolveCommanders resolves a list of commander names to full card data, verifies
// each is Commander-legal, and — for a two-card list — verifies the pair may legally
// share the command zone. The returned color identity is the union across all
// commanders (sorted in WUBRG order). A single non-partner commander is the normal
// case; partner pairs return both cards.
func (a *Analyzer) ResolveCommanders(ctx context.Context, names []string) ([]ResolvedCommander, error) {
	if a.cards == nil {
		return nil, ErrCardData
	}
	if len(names) == 0 {
		return nil, ErrBuildCommanderNotFound
	}
	cleaned := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		key := normalizeCardName(name)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		cleaned = append(cleaned, name)
	}
	if len(cleaned) == 0 {
		return nil, ErrBuildCommanderNotFound
	}

	catalog, err := a.cards.Lookup(ctx, cleaned)
	if err != nil {
		return nil, err
	}
	resolved := make([]ResolvedCommander, 0, len(cleaned))
	for _, name := range cleaned {
		card, ok := catalog[strings.ToLower(strings.TrimSpace(name))]
		if !ok {
			for key, value := range catalog {
				if normalizeCardName(key) == normalizeCardName(name) {
					card, ok = value, true
					break
				}
			}
		}
		if !ok {
			return nil, ErrBuildCommanderNotFound
		}
		if !hasUsableCardData(card) {
			return nil, ErrCardData
		}
		if card.Legalities["commander"] != "legal" {
			return nil, ErrCommanderNotLegal
		}
		if !isLegendaryCreature(card) {
			return nil, ErrCommanderNotLegal
		}
		resolved = append(resolved, ResolvedCommander{
			Name:          card.Name,
			Card:          card,
			ColorIdentity: sortedColors(card.ColorIdentity),
			IsPartner:     hasPartnerKeyword(card),
		})
	}

	if len(resolved) == 2 && !validCommanderPair(resolved[0].Card, resolved[1].Card) {
		return nil, ErrCommanderPairInvalid
	}
	if len(resolved) > 2 {
		return nil, ErrCommanderPairInvalid
	}
	return resolved, nil
}

// UnionColorIdentity returns the merged, WUBRG-ordered color identity of a set of
// resolved commanders (empty slice for a colorless commander).
func (a *Analyzer) UnionColorIdentity(commanders []ResolvedCommander) []string {
	colors := make(map[string]struct{})
	for _, commander := range commanders {
		for _, color := range commander.Card.ColorIdentity {
			colors[color] = struct{}{}
		}
	}
	list := make([]string, 0, len(colors))
	for color := range colors {
		list = append(list, color)
	}
	return sortedColors(list)
}

// weightedCommanderName picks a commander with probability inversely proportional to
// its deck count, so unpopular commanders surface more often than top staples. A
// deck count of 0 (missing data) is treated as the most unpopular, not the reverse.
func (a *Analyzer) weightedCommanderName(rankings []edhrec.CommanderRanking) string {
	if len(rankings) == 0 {
		return ""
	}
	total := 0.0
	weights := make([]float64, len(rankings))
	for i, item := range rankings {
		w := 1.0 / float64(item.DeckCount+1)
		weights[i] = w
		total += w
	}
	if total <= 0 {
		return rankings[a.rand.Intn(len(rankings))].Name
	}
	roll := a.rand.Float64() * total
	for i, w := range weights {
		roll -= w
		if roll <= 0 {
			return rankings[i].Name
		}
	}
	return rankings[len(rankings)-1].Name
}

// isLegendaryCreature reports whether a card's type line marks it a legendary
// creature (a prerequisite for the command zone). Multi-faced cards are checked
// across all faces.
func isLegendaryCreature(card cardcatalog.Card) bool {
	lines := []string{card.TypeLine}
	for _, face := range card.Faces {
		lines = append(lines, face.TypeLine)
	}
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), "legendary") && strings.Contains(strings.ToLower(line), "creature") {
			return true
		}
	}
	return false
}

// hasPartnerKeyword reports whether any face of a card carries a keyword that allows
// it to pair with a second commander.
func hasPartnerKeyword(card cardcatalog.Card) bool {
	text := strings.ToLower(card.OracleText)
	for _, face := range card.Faces {
		text += " " + strings.ToLower(face.OracleText)
	}
	return strings.Contains(text, "partner") ||
		strings.Contains(text, "friends forever") ||
		strings.Contains(text, "choose a background")
}

// validCommanderPair reports whether two commanders may share the command zone. This
// is a deliberate basic check: both must have a partner relationship (Partner,
// Friends Forever, or the "Choose a Background" mechanic). It is not a full rules
// implementation of every pairing nuance.
func validCommanderPair(left, right cardcatalog.Card) bool {
	return hasPartnerKeyword(left) && hasPartnerKeyword(right)
}

func sortedColors(colors []string) []string {
	list := make([]string, 0, len(colors))
	list = append(list, colors...)
	sort.Strings(list)
	return list
}

package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"powerlevel/internal/providers/cardcatalog"
	"powerlevel/internal/providers/edhrec"
	"powerlevel/internal/service/construction"
)

// ErrBuildCommanderNotFound is returned when the requested commander cannot be
// resolved to a usable Scryfall card.
var ErrBuildCommanderNotFound = errors.New("commander card was not found")

// seenRecycleAt is how many seen entries the front-end may accumulate before the
// server stops honoring `seen` as a hard exclusion. Below this, skipped cards stay
// hidden; at or above it, they are allowed to cycle back into later batches so a
// long draft never runs itself dry by skipping. Chosen cards remain excluded.
const seenRecycleAt = 20

// BuildCandidate is one suggested card the guided builder may offer the user.
type BuildCandidate struct {
	Name        string  `json:"name"`
	Synergy     float64 `json:"synergy"`
	Inclusion   float64 `json:"inclusion_rate"`
	Fills       []string `json:"fills"` // construction gap ids this card would help fill
	Card        cardcatalog.Card `json:"card"`
	SourceURL   string  `json:"source_url,omitempty"`
}

// BuildSuggestRequest seeds the guided builder: the commander name plus the cards
// already drafted, so the suggestion logic can avoid repeats and target gaps.
type BuildSuggestRequest struct {
	Commander string   `json:"commander"`
	Chosen    []string `json:"chosen"` // card names already added to the draft
	Seen      []string `json:"seen"`   // card names already shown (chosen or skipped)
	Count     int      `json:"count"`
}

// BuildSuggestResponse is a batch of scored candidates plus the commander's color
// identity (needed client-side to filter the basic-land quick add).
type BuildSuggestResponse struct {
	CommanderName string          `json:"commander_name"`
	ColorIdentity []string        `json:"color_identity"`
	Candidates    []BuildCandidate `json:"candidates"`
}

// buildGapWeights maps a construction metric id to a multiplier that prioritizes
var buildGapWeights = map[string]float64{
	"lands":             2.0,
	"plan":              1.2,
	"ramp":              1.5,
	"draw_discard":      1.3,
	"single_interaction": 1.1,
	"mass_interaction":   1.0,
}

// gapTypeQueries maps a construction metric id to the Scryfall type-line fragment
// (already URL-safe, no spaces) used to find more cards that fill that gap once
// the EDHREC pool is exhausted. "plan" is the generic synergy bucket, so it has no
// type constraint; it falls back to any legal card in the commander's colors.
var gapTypeQueries = map[string]string{
	"lands":              "t:land",
	"ramp":               "(t:artifact or t:creature)",
	"draw_discard":       "(t:sorcery or t:instant)",
	"single_interaction": "t:instant",
	"mass_interaction":   "t:sorcery",
}

// edhrecPoolCache memoizes the flattened, legality/color-filtered candidate pool per
// commander key. The EDHREC fetch and per-card Scryfall resolution are the two slow
// parts of BuildSuggest; caching them means repeated "换一批" calls for the same
// commander only re-rank and re-filter locally instead of hitting the network again.
type edhrecPoolCache struct {
	mu      sync.Mutex
	entries map[string]cachedPool
	ttl     time.Duration
}

type cachedPool struct {
	pool      []edhrecPoolCard
	expiresAt time.Time
}

func newEdhrecPoolCache(ttl time.Duration) *edhrecPoolCache {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &edhrecPoolCache{entries: make(map[string]cachedPool), ttl: ttl}
}

func (c *edhrecPoolCache) get(key string, now time.Time) ([]edhrecPoolCard, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok || !now.Before(entry.expiresAt) {
		return nil, false
	}
	return entry.pool, true
}

func (c *edhrecPoolCache) set(key string, pool []edhrecPoolCard, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Best-effort cap: keep the cache from growing unbounded across many commanders.
	if len(c.entries) >= 64 {
		for k := range c.entries {
			delete(c.entries, k)
			break
		}
	}
	c.entries[key] = cachedPool{pool: pool, expiresAt: now.Add(c.ttl)}
}

// BuildSuggest returns up to `count` scored card candidates for a guided deck
// build around the given commander. It pulls the EDHREC recommendation pool,
// filters out already-chosen, off-color, and non-Commander-legal cards, then ranks
// survivors by a blend of EDHREC synergy and how strongly each card fills the
// draft's current construction gaps. This is the same recommendation source the
// analysis flow already uses, so the builder and the analyzer agree on synergy.
func (a *Analyzer) BuildSuggest(ctx context.Context, request BuildSuggestRequest) (BuildSuggestResponse, error) {
	commanderName := strings.TrimSpace(request.Commander)
	if commanderName == "" {
		return BuildSuggestResponse{}, errors.New("commander name is required")
	}
		if a.edhrec == nil || a.cards == nil {
			return BuildSuggestResponse{}, ErrCardData
		}
		count := request.Count
		if count <= 0 {
			count = 20
		}
		if count > 50 {
			count = 50
		}

	// Resolve the commander first: we need its color identity and legality to gate
	// every candidate, and its name to key the EDHREC recommendation pool.
	commander, err := a.LookupCard(ctx, commanderName)
	if err != nil {
		if errors.Is(err, ErrAddCardNotFound) {
			return BuildSuggestResponse{}, ErrBuildCommanderNotFound
		}
		return BuildSuggestResponse{}, err
	}
	if !hasUsableCardData(commander) {
		return BuildSuggestResponse{}, ErrCardData
	}
	if commander.Legalities["commander"] != "legal" {
		return BuildSuggestResponse{}, fmt.Errorf("%s is not legal as a Commander", commander.Name)
	}
	commanderIdentity := map[string]struct{}{}
	for _, color := range commander.ColorIdentity {
		commanderIdentity[color] = struct{}{}
	}

	chosen := map[string]struct{}{}
	for _, name := range request.Chosen {
		chosen[normalizeCardName(name)] = struct{}{}
	}
	chosen[normalizeCardName(commander.Name)] = struct{}{}

	// Everything already surfaced this session (chosen or skipped) is excluded too,
	// so "换一批" never re-offers a card the user has already seen. This is a
	// stateless session keyed by the front-end's running `seen` list.
	seen := map[string]struct{}{}
	for _, name := range request.Seen {
		seen[normalizeCardName(name)] = struct{}{}
	}

	// Load the EDHREC recommendation pool for this commander, memoized per commander
	// key so repeated "换一批" calls don't re-fetch EDHREC or re-resolve every card.
	cacheKey := "edhrec:" + slugify(commander.Name)
	var pool []edhrecPoolCard
	if cached, ok := a.buildPoolCache.get(cacheKey, time.Now()); ok {
		pool = cached
	} else {
		groups, _, err := a.edhrec.Recommend(ctx, slugify(commander.Name), 60)
		if err != nil {
			return BuildSuggestResponse{}, fmt.Errorf("load EDHREC recommendations: %w", err)
		}
		pool = a.buildPool(ctx, groups, commanderIdentity)
		a.buildPoolCache.set(cacheKey, pool, time.Now())
	}

	// Re-filter the cached pool against this request's chosen+seen, then rank.
	// `seen` only excludes a card below the first `seenRecycleAt` entries; past that
	// many skips we let seen cards cycle back in, so "换一批" never dead-ends just
	// because the user skipped a lot of cards early on. Chosen cards are never
	// re-offered.
	var filtered []edhrecPoolCard
	for _, item := range pool {
		key := normalizeCardName(item.name)
		if _, used := chosen[key]; used {
			continue
		}
		if _, shown := seen[key]; shown && len(seen) <= seenRecycleAt {
			continue
		}
		filtered = append(filtered, item)
	}

	// When the EDHREC pool is too small to keep offering fresh cards (a shallow
	// commander page that only yields ~20 unique legal cards), top it up with
	// Scryfall cards in the commander's colors so the draft can keep growing.
	if len(filtered) < count {
		backfill := a.scryfallBackfill(ctx, commander, commanderIdentity, chosen, seen, count-len(filtered))
		filtered = append(filtered, backfill...)
	}

	// Rank by gap-fill priority × synergy. A card that fills no gap still gets a
	// baseline synergy score so it can appear once the category is satisfied.
	gapMult := a.currentGapMultipliers(ctx, commander, chosen)
	sort.SliceStable(filtered, func(i, j int) bool {
		return a.cardBuildScore(filtered[i], gapMult) > a.cardBuildScore(filtered[j], gapMult)
	})

	// Select `count` candidates while keeping their primary construction role (gap
	// category) spread out: pick the top card of each distinct role first, then fill
	// any remaining slots from whatever is left, so a batch of 3 doesn't return three
	// cards that all cover the same role (e.g. all ramp).
	selected := diverseSelection(filtered, gapMult, count)

	candidates := make([]BuildCandidate, 0, len(selected))
	for _, item := range selected {
		candidates = append(candidates, BuildCandidate{
			Name:      item.name,
			Synergy:   item.synergy,
			Inclusion: item.inclusion,
			Fills:     classifyIDs(construction.Classify(item.card)),
			Card:      item.card,
			SourceURL: item.source,
		})
	}

	identity := make([]string, 0, len(commanderIdentity))
	for color := range commanderIdentity {
		identity = append(identity, color)
	}
	sort.Strings(identity)
	return BuildSuggestResponse{CommanderName: commander.Name, ColorIdentity: identity, Candidates: candidates}, nil
}

type edhrecPoolCard struct {
	name      string
	synergy   float64
	inclusion float64
	source    string
	card      cardcatalog.Card
}

// resolveCandidate fetches a card's Scryfall payload; a lookup miss means we can't
// gate or display it, so it is dropped from the pool (not an error for the caller).
func (a *Analyzer) resolveCandidate(ctx context.Context, name string) (cardcatalog.Card, bool) {
	card, err := a.LookupCard(ctx, name)
	if err != nil || !hasUsableCardData(card) {
		return cardcatalog.Card{}, false
	}
	return card, true
}

// resolveCardCached is resolveCandidate but bounded by a tiny in-process memo so the
// gap-scoring pass over already-chosen cards doesn't re-hit Scryfall for the same
// names on every batch. Chosen-card identity is stable within a request.
var resolveMemo struct {
	sync.Mutex
	cards map[string]cardcatalog.Card
}

func (a *Analyzer) resolveCardCached(ctx context.Context, name string) (cardcatalog.Card, bool) {
	resolveMemo.Lock()
	if resolveMemo.cards == nil {
		resolveMemo.cards = make(map[string]cardcatalog.Card)
	}
	key := strings.ToLower(strings.TrimSpace(name))
	if card, ok := resolveMemo.cards[key]; ok {
		resolveMemo.Unlock()
		return card, true
	}
	resolveMemo.Unlock()

	card, ok := a.resolveCandidate(ctx, name)
	if ok {
		resolveMemo.Lock()
		if resolveMemo.cards == nil {
			resolveMemo.cards = make(map[string]cardcatalog.Card)
		}
		resolveMemo.cards[key] = card
		if len(resolveMemo.cards) > 512 {
			for k := range resolveMemo.cards {
				delete(resolveMemo.cards, k)
				break
			}
		}
		resolveMemo.Unlock()
	}
	return card, ok
}

// buildPool flattens EDHREC groups into a legality/color-filtered, deduplicated pool
// of resolved cards. It is the expensive part (network + Scryfall) and its result is
// memoized per commander by BuildSuggest. Every dedupe key uses normalizeCardName so
// a split card "X // Y" and its front face "X" never both survive.
func (a *Analyzer) buildPool(ctx context.Context, groups []edhrec.Group, commanderIdentity map[string]struct{}) []edhrecPoolCard {
	pool := make([]edhrecPoolCard, 0)
	seen := map[string]struct{}{}
	for _, group := range groups {
		for _, rec := range group.Cards {
			key := normalizeCardName(rec.Name)
			if _, dup := seen[key]; dup {
				continue
			}
			card, ok := a.resolveCandidate(ctx, rec.Name)
			if !ok {
				continue
			}
			if card.Legalities["commander"] != "legal" {
				continue
			}
			if !colorsAllowed(card.ColorIdentity, commanderIdentity) {
				continue
			}
			if isBasicLandType(card) {
				continue
			}
			if isNonDeckCardType(card) {
				continue
			}
			seen[key] = struct{}{}
			pool = append(pool, edhrecPoolCard{name: card.Name, synergy: rec.Synergy, inclusion: rec.InclusionRate, source: rec.SourceURL, card: card})
		}
	}
	return pool
}

// currentGapMultipliers computes, per construction category, how under-supplied the
// current draft is. It returns 1.0 for a satisfied category so scoring degrades to
// pure synergy once a slot is filled.
func (a *Analyzer) currentGapMultipliers(ctx context.Context, commander cardcatalog.Card, chosen map[string]struct{}) map[string]float64 {
	mult := map[string]float64{}
	inputs := []construction.InputCard{{Name: commander.Name, Quantity: 1, Card: commander}}
	for name := range chosen {
		card, ok := a.resolveCardCached(ctx, name)
		if !ok {
			continue
		}
		inputs = append(inputs, construction.InputCard{Name: card.Name, Quantity: 1, Card: card})
	}
	report := construction.Build(inputs)
	for _, metric := range report.Metrics {
		weight := buildGapWeights[metric.ID]
		if weight == 0 {
			weight = 1.0
		}
		if metric.Status == "short" {
			// Larger gap → larger multiplier, so the scarcest category wins.
			mult[metric.ID] = weight * (1.0 + 0.2*float64(metric.Gap))
		} else {
			mult[metric.ID] = 1.0
		}
	}
	return mult
}

// classifyIDs extracts the metric ids a card fills, as a stable string slice.
func classifyIDs(matches []construction.Match) []string {
	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		ids = append(ids, match.ID)
	}
	return ids
}

// primaryRole returns the single construction category that best represents a card
// for diversity purposes. It uses the category the card fills that currently carries
// the largest gap multiplier (i.e. what the draft most needs from it); a card that
// fills nothing falls back to an empty role. Cards whose only role is "lands" (basic
// land quick-add territory) are treated distinctly so they never crowd a batch.
func primaryRole(item edhrecPoolCard, gapMult map[string]float64) string {
	matches := construction.Classify(item.card)
	best := ""
	bestScore := 0.0
	for _, match := range matches {
		mult := gapMult[match.ID]
		if mult == 0 {
			mult = 1.0
		}
		if mult > bestScore {
			bestScore = mult
			best = match.ID
		}
	}
	return best
}

// diverseSelection spreads `count` candidates across distinct primary roles. It walks
// the already sorted pool once, taking the first card of each unseen role, then tops
// up any remaining slots from the pool in original order. Deterministic.
func diverseSelection(pool []edhrecPoolCard, gapMult map[string]float64, count int) []edhrecPoolCard {
	if count <= 0 {
		return nil
	}
	if count >= len(pool) {
		return pool
	}
	selected := make([]edhrecPoolCard, 0, count)
	usedRole := map[string]struct{}{}
	rest := make([]edhrecPoolCard, 0, len(pool))
	for _, item := range pool {
		if len(selected) >= count {
			break
		}
		role := primaryRole(item, gapMult)
		if role == "" {
			rest = append(rest, item)
			continue
		}
		if _, taken := usedRole[role]; taken {
			rest = append(rest, item)
			continue
		}
		usedRole[role] = struct{}{}
		selected = append(selected, item)
	}
	// Fill remaining slots from the leftover pool in original (score desc) order.
	for _, item := range rest {
		if len(selected) >= count {
			break
		}
		selected = append(selected, item)
	}
	return selected
}

// cardBuildScore ranks a candidate: gap-fill multipliers for every category the card
// fills (summed), times a small synergy weighting. A card filling a scarce category
// outscores a high-synergy card that fills nothing.
func (a *Analyzer) cardBuildScore(item edhrecPoolCard, gapMult map[string]float64) float64 {
	fill := 0.0
	for _, match := range construction.Classify(item.card) {
		mult := gapMult[match.ID]
		if mult == 0 {
			mult = 1.0
		}
		fill += mult
	}
	// Synergy stays on a comparable scale to fill multipliers; weight the gap-fill
	// intent above raw synergy so the builder actually closes construction gaps.
	return fill + (item.synergy / 100.0)
}

// scryfallBackfill pulls gap-filling cards from Scryfall when the EDHREC pool is
// exhausted. It queries by commander color identity plus a type constraint from
// gapTypeQueries, filtered to Commander-legal cards not already chosen/seen, and
// returns them as edhrecPoolCard entries with zero synergy (they ranked purely on
// whether they fill the draft's current gaps).
func (a *Analyzer) scryfallBackfill(ctx context.Context, commander cardcatalog.Card, commanderIdentity map[string]struct{}, chosen, seen map[string]struct{}, limit int) []edhrecPoolCard {
	var colors []string
	for color := range commanderIdentity {
		colors = append(colors, color)
	}
	sort.Strings(colors)
	ci := strings.ToLower(strings.Join(colors, ""))
	if ci == "" {
		ci = "c"
	}

	results := make([]edhrecPoolCard, 0, limit)
	collected := map[string]struct{}{}
	for _, metric := range targetsOrder {
		if len(results) >= limit {
			return results
		}
		queryType := gapTypeQueries[metric]
		q := "legal:commander ci:" + ci
		if queryType != "" {
			q += " " + queryType
		}
		// Ask for far more than `limit` so we can skip basic lands, supplemental
		// types, and already-seen cards while still filling the request. Post-filter
		// survivors are typically a small fraction of the raw results, so request
		// enough raw cards to survive the filtering.
		cards, err := a.cards.Search(ctx, q, 700)
		if err != nil {
			continue // one gap's search failing should not abort the whole backfill
		}
		for _, card := range cards {
			key := normalizeCardName(card.Name)
			if _, used := chosen[key]; used {
				continue
			}
			if _, shown := seen[key]; shown {
				continue
			}
			if _, dup := collected[key]; dup {
				continue
			}
			if isBasicLandType(card) {
				continue
			}
			if isNonDeckCardType(card) {
				continue
			}
			if !hasUsableCardData(card) {
				continue
			}
			if card.Legalities["commander"] != "legal" {
				continue
			}
			if !colorsAllowed(card.ColorIdentity, commanderIdentity) {
				continue
			}
			collected[key] = struct{}{}
			results = append(results, edhrecPoolCard{name: card.Name, card: card})
			if len(results) >= limit {
				return results
			}
		}
	}
	return results
}

// isBasicLandType reports whether a card is a basic land (Plains, Forest, ...),
// which the builder offers via the quick-add buttons instead of the suggestion pool.
func isBasicLandType(card cardcatalog.Card) bool {
	line := strings.ToLower(card.TypeLine)
	return strings.Contains(line, "basic") && strings.Contains(line, "land")
}

// isNonDeckCardType reports whether a card is a sealed-only supplemental type that
// is never legal in a constructed Commander deck (in practice), so it should not be
// offered by the builder: Attractions (Unfinity), Conspiracies (Conspiracy sets),
// and Stickers. Stickers and Attractions also frequently share the "sticker" text,
// so keying off the type line only would miss Sticker non-card sheets; the label
// "Sticker" appears as both a card type and a sticker ticket on these.
func isNonDeckCardType(card cardcatalog.Card) bool {
	line := strings.ToLower(card.TypeLine)
	return strings.Contains(line, "attraction") || strings.Contains(line, "conspiracy") || strings.Contains(line, "sticker")
}

// targetsOrder mirrors construction.targets order; gapTypeQueries is keyed by it.
var targetsOrder = []string{"lands", "ramp", "single_interaction", "mass_interaction", "draw_discard", "plan"}

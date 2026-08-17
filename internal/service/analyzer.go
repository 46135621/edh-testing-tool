package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"

	"powerlevel/internal/deck"
	"powerlevel/internal/providers/cardcatalog"
	"powerlevel/internal/providers/commandersalt"
	"powerlevel/internal/providers/edhrec"
	"powerlevel/internal/providers/spellbook"
)

type EDHAnalyzer interface {
	Analyze(context.Context, deck.Deck) (map[string]any, error)
}

type CardCatalog interface {
	Lookup(context.Context, []string) (map[string]cardcatalog.Card, error)
}

type Spellbook interface {
	Search(context.Context, []string, int) ([]spellbook.Combo, error)
}

type EDHRecommender interface {
	Recommend(context.Context, string, int) ([]edhrec.Group, []string, error)
}

type Analyzer struct {
	commanderSalt   *commandersalt.Client
	edh             EDHAnalyzer
	cards           CardCatalog
	spellbook       Spellbook
	edhrec          EDHRecommender
	providerTimeout time.Duration
	requestTimeout  time.Duration
	cacheTTL        time.Duration
	partialCacheTTL time.Duration
	cache           *analysisCache
	requests        singleflight.Group
}

func NewAnalyzer(
	commanderSalt *commandersalt.Client,
	edh EDHAnalyzer,
	cards CardCatalog,
	spellbookClient Spellbook,
	edhrecClient EDHRecommender,
	providerTimeout time.Duration,
	requestTimeout time.Duration,
	cacheTTL time.Duration,
	partialCacheTTL time.Duration,
	cacheMaxEntries int,
) *Analyzer {
	return &Analyzer{
		commanderSalt:   commanderSalt,
		edh:             edh,
		cards:           cards,
		spellbook:       spellbookClient,
		edhrec:          edhrecClient,
		providerTimeout: providerTimeout,
		requestTimeout:  requestTimeout,
		cacheTTL:        cacheTTL,
		partialCacheTTL: partialCacheTTL,
		cache:           newAnalysisCache(cacheMaxEntries),
	}
}

func (a *Analyzer) Analyze(ctx context.Context, sourceURL, sourceID string) (Analysis, error) {
	if cached, ok := a.cache.get(sourceID, time.Now()); ok {
		return cached, nil
	}

	resultChannel := a.requests.DoChan(sourceID, func() (any, error) {
		if cached, ok := a.cache.get(sourceID, time.Now()); ok {
			return cached, nil
		}
		sharedCtx, cancel := context.WithTimeout(context.Background(), a.requestTimeout)
		defer cancel()
		analysis, err := a.analyze(sharedCtx, sourceURL, sourceID)
		if err == nil {
			ttl := a.cacheTTL
			if analysis.Status == "partial" {
				ttl = a.partialCacheTTL
			}
			if ttl > 0 {
				a.cache.set(sourceID, analysis, time.Now().Add(ttl))
			}
		}
		return analysis, err
	})

	select {
	case <-ctx.Done():
		return Analysis{}, ctx.Err()
	case result := <-resultChannel:
		if result.Err != nil {
			return Analysis{}, result.Err
		}
		return cloneAnalysis(result.Val.(Analysis)), nil
	}
}

func (a *Analyzer) analyze(ctx context.Context, sourceURL, sourceID string) (Analysis, error) {
	commanderCtx, cancelCommander := context.WithTimeout(ctx, a.providerTimeout)
	commanderResult, commanderErr := a.commanderSalt.Analyze(commanderCtx, sourceURL, sourceID)
	cancelCommander()
	if commanderErr != nil {
		return Analysis{}, fmt.Errorf("load deck through CommanderSalt: %w", commanderErr)
	}

	analysis := Analysis{
		Status:  "success",
		Results: map[string]ProviderResult{"commandersalt": {Status: "success", Metrics: commanderResult.Metrics}},
	}
	analysis.Deck = summarize(commanderResult.Deck)
	if a.cards != nil {
		cardCtx, cancelCards := context.WithTimeout(ctx, a.providerTimeout)
		catalog, cardErr := a.cards.Lookup(cardCtx, deckNames(commanderResult.Deck))
		cancelCards()
		if cardErr != nil {
			analysis.Warnings = append(analysis.Warnings, "卡牌图片与详情暂时无法加载。")
		} else {
			analysis.DeckCards = buildDisplayCards(commanderResult.Deck, catalog)
		}
	}

	if a.edhrec != nil && a.cards != nil && len(commanderResult.Deck.Commanders) > 0 {
		recCtx, cancelRec := context.WithTimeout(ctx, a.providerTimeout)
		groups, keywords, recErr := a.edhrec.Recommend(recCtx, slugify(commanderResult.Deck.Commanders[0].Name), 20)
		cancelRec()
		if recErr != nil {
			analysis.Warnings = append(analysis.Warnings, "EDHREC 主将推荐暂时无法加载。")
		} else {
			analysis.RecommendationKeywords = keywords
			candidateNames := recommendationNames(groups)
			candidateCtx, cancelCandidates := context.WithTimeout(ctx, a.providerTimeout)
			catalog, catalogErr := a.cards.Lookup(candidateCtx, candidateNames)
			cancelCandidates()
			if catalogErr == nil {
				analysis.Recommendations = filterRecommendationGroups(groups, catalog, analysis.DeckCards, keywords, 8)
			}
		}
	}

	if a.spellbook != nil {
		comboCtx, cancelCombos := context.WithTimeout(ctx, a.providerTimeout)
		found, comboErr := a.spellbook.Search(comboCtx, deckNames(commanderResult.Deck), 12)
		cancelCombos()
		if comboErr != nil {
			analysis.Warnings = append(analysis.Warnings, "Commander Spellbook 组合暂时无法加载。")
		} else {
			analysis.Combos, analysis.RelatedCards = buildCombos(found, analysis.DeckCards)
		}
	}

	edhCtx, cancelEDH := context.WithTimeout(ctx, a.providerTimeout)
	edhMetrics, edhErr := a.edh.Analyze(edhCtx, commanderResult.Deck)
	cancelEDH()
	if edhErr != nil {
		analysis.Status = "partial"
		analysis.Results["edhpowerlevel"] = failure(edhErr)
		analysis.Warnings = append(analysis.Warnings, "EDH Power Level 分析失败，CommanderSalt 结果仍然可用。")
	} else {
		analysis.Results["edhpowerlevel"] = ProviderResult{Status: "success", Metrics: edhMetrics}
	}
	return analysis, nil
}

func slugify(value string) string {
	value = strings.ToLower(value)
	var builder strings.Builder
	lastDash := false
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			builder.WriteRune(char)
			lastDash = false
		} else if !lastDash && builder.Len() > 0 {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func recommendationNames(groups []edhrec.Group) []string {
	seen := make(map[string]struct{})
	var names []string
	for _, group := range groups {
		for _, item := range group.Cards {
			key := strings.ToLower(item.Name)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			names = append(names, item.Name)
		}
	}
	return names
}

func filterRecommendationGroups(groups []edhrec.Group, catalog map[string]cardcatalog.Card, deckCards []DisplayCard, keywords []string, limit int) []RecommendationGroup {
	existing := make(map[string]struct{}, len(deckCards))
	commanderColors := make(map[string]struct{})
	for _, item := range deckCards {
		existing[strings.ToLower(item.Card.Name)] = struct{}{}
		if item.Commander {
			for _, color := range item.Card.ColorIdentity {
				commanderColors[color] = struct{}{}
			}
		}
	}
	var result []RecommendationGroup
	for _, group := range groups {
		output := RecommendationGroup{Header: group.Header, Tag: group.Tag}
		for _, item := range group.Cards {
			card, ok := catalog[strings.ToLower(item.Name)]
			if !ok {
				continue
			}
			if _, exists := existing[strings.ToLower(card.Name)]; exists {
				continue
			}
			if card.Legalities["commander"] != "legal" || !colorsAllowed(card.ColorIdentity, commanderColors) {
				continue
			}
			reason := "Recommended by EDHREC in " + group.Header
			output.Cards = append(output.Cards, RecommendedCard{Card: card, Synergy: item.Synergy, InclusionRate: item.InclusionRate, Reason: reason, SourceURL: item.SourceURL, Keywords: keywords})
			if len(output.Cards) >= limit {
				break
			}
		}
		if len(output.Cards) > 0 {
			result = append(result, output)
		}
	}
	return result
}

func colorsAllowed(colors []string, allowed map[string]struct{}) bool {
	for _, color := range colors {
		if _, ok := allowed[color]; !ok {
			return false
		}
	}
	return true
}

func buildCombos(found []spellbook.Combo, deckCards []DisplayCard) ([]Combo, []DisplayCard) {
	cardsByName := make(map[string]DisplayCard, len(deckCards))
	for _, item := range deckCards {
		cardsByName[strings.ToLower(item.Card.Name)] = item
	}
	seenRelated := make(map[string]struct{})
	var combos []Combo
	var related []DisplayCard
	for _, source := range found {
		combo := Combo{Name: source.Name, Result: source.Result, Steps: source.Steps, Sources: []string{"commander_spellbook"}, SourceURL: source.SourceURL}
		for _, component := range source.Components {
			item, ok := cardsByName[strings.ToLower(component.Name)]
			if !ok {
				item = DisplayCard{Card: cardcatalog.Card{OracleID: component.OracleID, Name: component.Name, ImageNormal: component.ImageNormal, ImageSmall: component.ImageSmall}, Quantity: 1}
			}
			combo.Components = append(combo.Components, item)
			key := strings.ToLower(component.Name)
			if _, ok := seenRelated[key]; !ok {
				seenRelated[key] = struct{}{}
				related = append(related, item)
			}
		}
		combos = append(combos, combo)
	}
	return combos, related
}

func deckNames(target deck.Deck) []string {
	names := make([]string, 0, len(target.Commanders)+len(target.Mainboard))
	for _, card := range target.Commanders {
		names = append(names, card.Name)
	}
	for _, card := range target.Mainboard {
		names = append(names, card.Name)
	}
	return names
}

func buildDisplayCards(target deck.Deck, catalog map[string]cardcatalog.Card) []DisplayCard {
	result := make([]DisplayCard, 0, len(target.Commanders)+len(target.Mainboard))
	appendCard := func(item deck.Card, commander bool) {
		card, ok := catalog[strings.ToLower(strings.TrimSpace(item.Name))]
		if !ok {
			card = cardcatalog.Card{Name: item.Name}
		}
		result = append(result, DisplayCard{Card: card, Quantity: item.Quantity, Commander: commander, Land: strings.Contains(strings.ToLower(card.TypeLine), "land")})
	}
	for _, item := range target.Commanders {
		appendCard(item, true)
	}
	for _, item := range target.Mainboard {
		appendCard(item, false)
	}
	return result
}

func summarize(target deck.Deck) DeckSummary {
	commanders := make([]string, 0, len(target.Commanders))
	for _, card := range target.Commanders {
		commanders = append(commanders, card.Name)
	}
	return DeckSummary{
		ID:         target.SourceID,
		Name:       target.Name,
		Commanders: commanders,
		CardCount:  target.CardCount(),
	}
}

func failure(err error) ProviderResult {
	code := "PROVIDER_ERROR"
	if errors.Is(err, context.DeadlineExceeded) {
		code = "PROVIDER_TIMEOUT"
	} else if errors.Is(err, context.Canceled) {
		code = "PROVIDER_CANCELED"
	}
	return ProviderResult{Status: "error", Error: &ProviderError{Code: code, Message: err.Error()}}
}

package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"powerlevel/internal/deck"
	"powerlevel/internal/providers/cardcatalog"
	"powerlevel/internal/service/construction"
)

var (
	ErrRemoveCardNotFound  = errors.New("remove card not found in mainboard")
	ErrCommanderSwap       = errors.New("commander replacement is not supported")
	ErrAddCardNotFound     = errors.New("added card was not found")
	ErrIllegalAddedCard    = errors.New("added card is not commander legal")
	ErrColorIdentity       = errors.New("added card is outside the commander color identity")
	ErrSingleton           = errors.New("added card would violate singleton rules")
	ErrSameCard            = errors.New("added and removed cards must differ")
	ErrCardData            = errors.New("card data is incomplete")
	errUnknownLandCategory = errors.New("unknown land category")
)

type SwapCard struct {
	Name string `json:"name"`
}

type SwapMetricDelta struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Before int    `json:"before"`
	After  int    `json:"after"`
	Delta  int    `json:"delta"`
	Target int    `json:"target"`
}

type SwapDeckState struct {
	CardCount          int                 `json:"card_count"`
	ConstructionReport construction.Report `json:"construction_report"`
}

type SwapLegality struct {
	Valid               bool     `json:"valid"`
	CardCountValid      bool     `json:"card_count_valid"`
	CommanderCountValid bool     `json:"commander_count_valid"`
	ColorIdentity       []string `json:"color_identity"`
	Issues              []string `json:"issues"`
}

type SwapComparison struct {
	Removed         SwapCard          `json:"removed"`
	Added           SwapCard          `json:"added"`
	Before          SwapDeckState     `json:"before"`
	After           SwapDeckState     `json:"after"`
	Deltas          []SwapMetricDelta `json:"deltas"`
	Legality        SwapLegality      `json:"legality"`
	UpdatedDecklist string            `json:"updated_decklist"`
	DeckRevision    string            `json:"deck_revision"`
}

func (a *Analyzer) CompareSwap(ctx context.Context, decklist, removeName, addName string) (SwapComparison, error) {
	base, err := deck.ParsePlainText(decklist)
	if err != nil {
		return SwapComparison{}, err
	}
	removeName = strings.TrimSpace(removeName)
	addName = strings.TrimSpace(addName)
	if removeName == "" || addName == "" {
		return SwapComparison{}, errors.New("remove and add card names are required")
	}
	if strings.EqualFold(removeName, addName) {
		return SwapComparison{}, ErrSameCard
	}
	for _, commander := range base.Commanders {
		if strings.EqualFold(commander.Name, removeName) {
			return SwapComparison{}, ErrCommanderSwap
		}
	}

	after := cloneDeck(base)
	removed := false
	for i := range after.Mainboard {
		if !strings.EqualFold(after.Mainboard[i].Name, removeName) {
			continue
		}
		removed = true
		after.Mainboard[i].Quantity--
		if after.Mainboard[i].Quantity == 0 {
			after.Mainboard = append(after.Mainboard[:i], after.Mainboard[i+1:]...)
		}
		break
	}
	if !removed {
		return SwapComparison{}, ErrRemoveCardNotFound
	}
	if a.cards == nil {
		return SwapComparison{}, ErrCardData
	}

	names := append(deckNames(base), addName)
	catalog, err := a.cards.Lookup(ctx, names)
	if err != nil {
		return SwapComparison{}, fmt.Errorf("lookup card data: %w", err)
	}
	added, ok := catalog[strings.ToLower(addName)]
	if !ok {
		return SwapComparison{}, ErrAddCardNotFound
	}
	if !hasUsableCardData(added) {
		return SwapComparison{}, ErrCardData
	}
	if added.Legalities["commander"] != "legal" {
		return SwapComparison{}, ErrIllegalAddedCard
	}
	commanderColors, colorList, err := commanderColorIdentity(base, catalog)
	if err != nil {
		return SwapComparison{}, err
	}
	if !colorsAllowed(added.ColorIdentity, commanderColors) {
		return SwapComparison{}, ErrColorIdentity
	}
	if !strings.EqualFold(removeName, added.Name) && deckContains(base, added.Name) && !allowsMultipleCopies(added) {
		return SwapComparison{}, ErrSingleton
	}
	after.Mainboard = addCard(after.Mainboard, added.Name)

	beforeInputs, err := constructionInputs(base, catalog)
	if err != nil {
		return SwapComparison{}, err
	}
	afterInputs, err := constructionInputs(after, catalog)
	if err != nil {
		return SwapComparison{}, err
	}
	beforeReport := construction.Build(beforeInputs)
	afterReport := construction.Build(afterInputs)
	updated := after.ExportPlainText()
	legality := basicLegality(after, catalog, colorList)
	return SwapComparison{
		Removed:         SwapCard{Name: removeName},
		Added:           SwapCard{Name: added.Name},
		Before:          SwapDeckState{CardCount: base.CardCount(), ConstructionReport: beforeReport},
		After:           SwapDeckState{CardCount: after.CardCount(), ConstructionReport: afterReport},
		Deltas:          metricDeltas(beforeReport, afterReport),
		Legality:        legality,
		UpdatedDecklist: updated,
		DeckRevision:    deckRevision(updated),
	}, nil
}

func cloneDeck(source deck.Deck) deck.Deck {
	cloned := source
	cloned.Commanders = append([]deck.Card(nil), source.Commanders...)
	cloned.Mainboard = append([]deck.Card(nil), source.Mainboard...)
	return cloned
}

func addCard(cards []deck.Card, name string) []deck.Card {
	for i := range cards {
		if strings.EqualFold(cards[i].Name, name) {
			cards[i].Quantity++
			return cards
		}
	}
	return append(cards, deck.Card{Name: name, Quantity: 1})
}

func deckContains(target deck.Deck, name string) bool {
	for _, card := range append(append([]deck.Card(nil), target.Commanders...), target.Mainboard...) {
		if strings.EqualFold(card.Name, name) && card.Quantity > 0 {
			return true
		}
	}
	return false
}

func hasUsableCardData(card cardcatalog.Card) bool {
	return card.Name != "" && (card.TypeLine != "" || card.OracleText != "" || len(card.Faces) > 0)
}

func allowsMultipleCopies(card cardcatalog.Card) bool {
	if strings.Contains(strings.ToLower(card.TypeLine), "basic land") {
		return true
	}
	text := strings.ToLower(card.OracleText)
	return strings.Contains(text, "a deck can have any number of cards named") || strings.Contains(text, "a deck can have up to")
}

func commanderColorIdentity(target deck.Deck, catalog map[string]cardcatalog.Card) (map[string]struct{}, []string, error) {
	colors := make(map[string]struct{})
	for _, commander := range target.Commanders {
		card, ok := catalog[strings.ToLower(commander.Name)]
		if !ok || !hasUsableCardData(card) {
			return nil, nil, ErrCardData
		}
		for _, color := range card.ColorIdentity {
			colors[color] = struct{}{}
		}
	}
	list := make([]string, 0, len(colors))
	for color := range colors {
		list = append(list, color)
	}
	sort.Strings(list)
	return colors, list, nil
}

func constructionInputs(target deck.Deck, catalog map[string]cardcatalog.Card) ([]construction.InputCard, error) {
	cards := append(append([]deck.Card(nil), target.Commanders...), target.Mainboard...)
	inputs := make([]construction.InputCard, 0, len(cards))
	for _, item := range cards {
		card, ok := catalog[strings.ToLower(item.Name)]
		if !ok || !hasUsableCardData(card) {
			return nil, ErrCardData
		}
		inputs = append(inputs, construction.InputCard{Name: card.Name, Quantity: item.Quantity, Card: card})
	}
	return inputs, nil
}

func metricDeltas(before, after construction.Report) []SwapMetricDelta {
	afterByID := make(map[string]construction.Metric, len(after.Metrics))
	for _, metric := range after.Metrics {
		afterByID[metric.ID] = metric
	}
	result := make([]SwapMetricDelta, 0, len(before.Metrics))
	for _, metric := range before.Metrics {
		next := afterByID[metric.ID]
		result = append(result, SwapMetricDelta{ID: metric.ID, Label: metric.Label, Before: metric.Actual, After: next.Actual, Delta: next.Actual - metric.Actual, Target: metric.Target})
	}
	return result
}

func basicLegality(target deck.Deck, catalog map[string]cardcatalog.Card, colors []string) SwapLegality {
	issues := make([]string, 0)
	cardCountValid := target.CardCount() == 100
	commanderCountValid := len(target.Commanders) > 0
	if !cardCountValid {
		issues = append(issues, fmt.Sprintf("牌组当前为 %d 张；标准 Commander 牌组通常为 100 张。", target.CardCount()))
	}
	if !commanderCountValid {
		issues = append(issues, "牌组缺少 Commander。")
	}
	counts := make(map[string]int)
	for _, item := range append(append([]deck.Card(nil), target.Commanders...), target.Mainboard...) {
		counts[strings.ToLower(item.Name)] += item.Quantity
	}
	for key, count := range counts {
		card, ok := catalog[key]
		if !ok || !hasUsableCardData(card) {
			issues = append(issues, "部分卡牌资料不完整。")
			continue
		}
		if card.Legalities["commander"] != "legal" {
			issues = append(issues, card.Name+" 不是 Commander 合法牌。")
		}
		if count > 1 && !allowsMultipleCopies(card) {
			issues = append(issues, card.Name+" 超过单卡一张限制。")
		}
	}
	return SwapLegality{Valid: len(issues) == 0, CardCountValid: cardCountValid, CommanderCountValid: commanderCountValid, ColorIdentity: colors, Issues: issues}
}

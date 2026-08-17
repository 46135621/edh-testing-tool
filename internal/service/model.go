package service

import (
	"context"

	"powerlevel/internal/deck"
	"powerlevel/internal/providers/cardcatalog"
)

type ProviderResult struct {
	Status  string         `json:"status"`
	Metrics map[string]any `json:"metrics,omitempty"`
	Error   *ProviderError `json:"error,omitempty"`
}

type ProviderError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type DeckSummary struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Commanders []string `json:"commanders"`
	CardCount  int      `json:"card_count"`
}

type DisplayCard struct {
	Card      cardcatalog.Card `json:"card"`
	Quantity  int              `json:"quantity"`
	Commander bool             `json:"commander,omitempty"`
	Land      bool             `json:"land,omitempty"`
}

type Combo struct {
	Name       string        `json:"name"`
	Components []DisplayCard `json:"components"`
	Result     string        `json:"result,omitempty"`
	Steps      []string      `json:"steps,omitempty"`
	Sources    []string      `json:"sources"`
	SourceURL  string        `json:"source_url,omitempty"`
}

type RecommendedCard struct {
	Card          cardcatalog.Card `json:"card"`
	Synergy       float64          `json:"synergy"`
	InclusionRate float64          `json:"inclusion_rate"`
	Reason        string           `json:"reason"`
	SourceURL     string           `json:"source_url"`
	Keywords      []string         `json:"keywords,omitempty"`
}

type RecommendationGroup struct {
	Header string            `json:"header"`
	Tag    string            `json:"tag"`
	Cards  []RecommendedCard `json:"cards"`
}

type Analysis struct {
	Status                 string                    `json:"status"`
	Deck                   DeckSummary               `json:"deck"`
	Results                map[string]ProviderResult `json:"results"`
	DeckCards              []DisplayCard             `json:"deck_cards,omitempty"`
	RelatedCards           []DisplayCard             `json:"related_cards,omitempty"`
	Combos                 []Combo                   `json:"combos,omitempty"`
	Recommendations        []RecommendationGroup     `json:"recommendations,omitempty"`
	RecommendationKeywords []string                  `json:"recommendation_keywords,omitempty"`
	Warnings               []string                  `json:"warnings,omitempty"`
}

type Provider interface {
	Name() string
	Analyze(context.Context, deck.Deck) (map[string]any, error)
}

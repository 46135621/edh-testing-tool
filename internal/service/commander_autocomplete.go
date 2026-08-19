package service

import (
	"context"
	"strings"
)

// SuggestCommanders returns up to `limit` card-name suggestions for a partial
// commander name, filtered to cards that are actually legal to run as a Commander.
// Autocomplete typeahead returns names only, so legality is confirmed with a batch
// lookup; suggestions that cannot be resolved to a legal Commander are dropped so
// the front-end never offers a dead-end.
func (a *Analyzer) SuggestCommanders(ctx context.Context, query string, limit int) ([]string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, ErrBuildCommanderNotFound
	}
	if a.cards == nil {
		return nil, ErrCardData
	}
	names, err := a.cards.Autocomplete(ctx, query)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(names) > limit {
		names = names[:limit]
	}
	catalog, err := a.cards.Lookup(ctx, names)
	if err != nil {
		return nil, err
	}
	suggestions := make([]string, 0, len(names))
	for _, name := range names {
		card, ok := catalog[strings.ToLower(strings.TrimSpace(name))]
		if !ok || !hasUsableCardData(card) {
			continue
		}
		if card.Legalities["commander"] != "legal" {
			continue
		}
		suggestions = append(suggestions, card.Name)
	}
	return suggestions, nil
}

// SuggestCards returns up to `limit` canonical card names for a partial name, suitable
// for the light deck editor's add-card field. Unlike SuggestCommanders it keeps every
// resolvable card (it is not Commander-legal filtered), so any card a user might paste
// into a decklist can be suggested; basic and multiple-copy cards are left in, since a
// decklist legitimately contains them.
func (a *Analyzer) SuggestCards(ctx context.Context, query string, limit int) ([]string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, ErrBuildCommanderNotFound
	}
	if a.cards == nil {
		return nil, ErrCardData
	}
	names, err := a.cards.Autocomplete(ctx, query)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(names) > limit {
		names = names[:limit]
	}
	catalog, err := a.cards.Lookup(ctx, names)
	if err != nil {
		return nil, err
	}
	suggestions := make([]string, 0, len(names))
	for _, name := range names {
		card, ok := catalog[strings.ToLower(strings.TrimSpace(name))]
		if !ok || !hasUsableCardData(card) {
			continue
		}
		suggestions = append(suggestions, card.Name)
	}
	return suggestions, nil
}

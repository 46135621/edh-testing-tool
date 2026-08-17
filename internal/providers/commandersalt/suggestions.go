package commandersalt

import (
	"encoding/json"
	"strings"
)

type Suggestions struct {
	Summary         string           `json:"summary,omitempty"`
	ShowSuggestions bool             `json:"show_suggestions"`
	Rationale       []SuggestionItem `json:"rationale"`
	Soften          []SuggestionItem `json:"soften"`
	Harden          []SuggestionItem `json:"harden"`
	RuleZero        []SuggestionItem `json:"rule_zero"`
}

type SuggestionItem struct {
	ID        string         `json:"id,omitempty"`
	Label     string         `json:"label,omitempty"`
	Why       string         `json:"why,omitempty"`
	Sentiment string         `json:"sentiment,omitempty"`
	Direction string         `json:"direction,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

type bracketSuggestions struct {
	ShowSuggestions bool `json:"showSuggestions"`
	Profile         struct {
		Summary   string           `json:"summary"`
		Rationale []SuggestionItem `json:"rationale"`
		Soften    []SuggestionItem `json:"soften"`
		Harden    []SuggestionItem `json:"harden"`
		RuleZero  []SuggestionItem `json:"ruleZero"`
	} `json:"profile"`
}

func parseSuggestions(payload map[string]json.RawMessage) (Suggestions, bool) {
	var best Suggestions
	var walk func(json.RawMessage)
	walk = func(raw json.RawMessage) {
		var object map[string]json.RawMessage
		if json.Unmarshal(raw, &object) != nil {
			return
		}
		if profileRaw, ok := object["profile"]; ok {
			var candidate bracketSuggestions
			candidate.ShowSuggestions = rawBool(object["showSuggestions"])
			if json.Unmarshal(profileRaw, &candidate.Profile) == nil {
				parsed := Suggestions{
					Summary:         strings.TrimSpace(candidate.Profile.Summary),
					ShowSuggestions: candidate.ShowSuggestions,
					Rationale:       normalizeItems(candidate.Profile.Rationale),
					Soften:          normalizeItems(candidate.Profile.Soften),
					Harden:          normalizeItems(candidate.Profile.Harden),
					RuleZero:        normalizeItems(candidate.Profile.RuleZero),
				}
				if suggestionScore(parsed) > suggestionScore(best) {
					best = parsed
				}
			}
		}
		for _, child := range object {
			walk(child)
		}
	}
	for _, raw := range payload {
		walk(raw)
	}
	return best, suggestionScore(best) > 0
}

func normalizeItems(items []SuggestionItem) []SuggestionItem {
	result := make([]SuggestionItem, 0, len(items))
	for _, item := range items {
		item.ID = strings.TrimSpace(item.ID)
		item.Label = strings.TrimSpace(item.Label)
		item.Why = strings.TrimSpace(item.Why)
		item.Sentiment = strings.TrimSpace(item.Sentiment)
		item.Direction = strings.TrimSpace(item.Direction)
		if item.ID == "" && item.Label == "" && item.Why == "" {
			continue
		}
		result = append(result, item)
	}
	return result
}

func suggestionScore(value Suggestions) int {
	score := len(value.Rationale) + len(value.Soften) + len(value.Harden) + len(value.RuleZero)
	if value.Summary != "" {
		score++
	}
	return score
}

func rawBool(raw json.RawMessage) bool {
	var value bool
	_ = json.Unmarshal(raw, &value)
	return value
}

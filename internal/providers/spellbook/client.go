package spellbook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const maxResponseSize = 6 << 20

type Combo struct {
	ID              string
	Name            string
	Components      []Component
	Result          string
	Steps           []string
	SourceURL       string
	ManaValueNeeded int
	// RequiresTemplates holds the numeric template IDs of any "requires" entries
	// (the site's `Ee.requirements` list determines which are acceptable).
	RequiresTemplates []int
	// ProducesFeatureIDs holds the numeric feature IDs the combo produces; the site
	// compares these against a fixed "game-defining" producer list.
	ProducesFeatureIDs []int
}

type Component struct {
	Name        string
	OracleID    string
	ImageNormal string
	ImageSmall  string
	Zone        string
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type response struct {
	Results []variant `json:"results"`
}

type variant struct {
	ID              string `json:"id"`
	Description     string `json:"description"`
	ManaValueNeeded int    `json:"manaValueNeeded"`
	BracketTag      string `json:"bracketTag"`
	Status          string `json:"status"`
	Uses            []struct {
		Card struct {
			Name                string `json:"name"`
			OracleID            string `json:"oracleId"`
			ImageUriFrontNormal string `json:"imageUriFrontNormal"`
			ImageUriFrontSmall  string `json:"imageUriFrontSmall"`
		} `json:"card"`
		ZoneLocations []string `json:"zoneLocations"`
	} `json:"uses"`
	Produces []struct {
		Feature struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"feature"`
	} `json:"produces"`
	Requires []struct {
		Quantity int `json:"quantity"`
		Template struct {
			ID int `json:"id"`
		} `json:"template"`
	} `json:"requires"`
}

func New(baseURL string, httpClient *http.Client) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), httpClient: httpClient}
}

func (c *Client) Search(ctx context.Context, names []string, limit int) ([]Combo, error) {
	if limit < 1 {
		limit = 12
	}
	seen := make(map[string]struct{})
	var combos []Combo
	for _, name := range prioritizeNames(names) {
		endpoint, _ := url.Parse(c.baseURL + "/variants/")
		query := endpoint.Query()
		// Query the front face: spellbook's search doesn't recognize "X // Y".
		query.Set("q", normalizeSpellbookName(name))
		query.Set("limit", "12")
		endpoint.RawQuery = query.Encode()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "PowerLevelAggregator/0.2")
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return combos, fmt.Errorf("request Commander Spellbook: %w", err)
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
		resp.Body.Close()
		if readErr != nil {
			return combos, readErr
		}
		if resp.StatusCode != http.StatusOK {
			return combos, fmt.Errorf("Commander Spellbook returned HTTP %d", resp.StatusCode)
		}
		var payload response
		if err := json.Unmarshal(data, &payload); err != nil {
			return combos, err
		}
		for _, item := range payload.Results {
			if _, ok := seen[item.ID]; ok {
				continue
			}
			if !allComponentsInDeck(item, names) {
				continue
			}
			seen[item.ID] = struct{}{}
			combo := Combo{ID: item.ID, SourceURL: "https://commanderspellbook.com/combo/" + item.ID, ManaValueNeeded: item.ManaValueNeeded}
			for _, use := range item.Uses {
				zone := ""
				if len(use.ZoneLocations) > 0 {
					zone = strings.Join(use.ZoneLocations, ",")
				}
				combo.Components = append(combo.Components, Component{Name: use.Card.Name, OracleID: use.Card.OracleID, ImageNormal: use.Card.ImageUriFrontNormal, ImageSmall: use.Card.ImageUriFrontSmall, Zone: zone})
			}
			for _, req := range item.Requires {
				combo.RequiresTemplates = append(combo.RequiresTemplates, req.Template.ID)
			}
			for _, p := range item.Produces {
				if p.Feature.ID != 0 {
					combo.ProducesFeatureIDs = append(combo.ProducesFeatureIDs, p.Feature.ID)
				}
			}
			parts := make([]string, 0, len(combo.Components))
			for _, part := range combo.Components {
				parts = append(parts, part.Name)
			}
			combo.Name = strings.Join(parts, " + ")
			produces := make([]string, 0, len(item.Produces))
			for _, p := range item.Produces {
				if p.Feature.Name != "" {
					produces = append(produces, p.Feature.Name)
				}
			}
			combo.Result = strings.Join(produces, ", ")
			for _, step := range strings.Split(item.Description, "\n") {
				if step = strings.TrimSpace(step); step != "" {
					combo.Steps = append(combo.Steps, step)
				}
			}
			combos = append(combos, combo)
			if len(combos) >= limit {
				return combos, nil
			}
		}
	}
	return combos, nil
}

func prioritizeNames(names []string) []string {
	keywords := []string{"twinflame", "dualcaster", "ghostly flicker", "naru meha", "dramatic reversal", "isochron", "thassa", "consultation", "tainted pact", "underworld breach", "lion's eye diamond"}
	var priority, rest []string
	for _, name := range names {
		lower := strings.ToLower(name)
		matched := false
		for _, keyword := range keywords {
			if strings.Contains(lower, keyword) {
				matched = true
				break
			}
		}
		if matched {
			priority = append(priority, name)
		} else {
			rest = append(rest, name)
		}
	}
	priority = append(priority, rest...)
	if len(priority) > 24 {
		priority = priority[:24]
	}
	return priority
}

func allComponentsInDeck(item variant, deckNames []string) bool {
	set := make(map[string]struct{}, len(deckNames))
	for _, name := range deckNames {
		// Index by front-face name so a deck entry "X // Y" matches both "X // Y"
		// and "X" (and vice versa) from the combo API. Display names are untouched.
		set[normalizeSpellbookName(name)] = struct{}{}
	}
	if len(item.Uses) < 2 {
		return false
	}
	for _, use := range item.Uses {
		if _, ok := set[normalizeSpellbookName(use.Card.Name)]; !ok {
			return false
		}
	}
	return true
}

// normalizeSpellbookName lowercases and collapses a name to its front face for
// matching; it does not affect any returned display/image data.
func normalizeSpellbookName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if index := strings.Index(name, " // "); index > 0 {
		name = strings.TrimSpace(name[:index])
	}
	return name
}

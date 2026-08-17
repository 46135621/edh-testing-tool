package cardcatalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	maxBatchSize    = 75
	maxResponseSize = 8 << 20
)

type CardFace struct {
	Name        string `json:"name"`
	PrintedName string `json:"printed_name,omitempty"`
	ManaCost    string `json:"mana_cost,omitempty"`
	TypeLine    string `json:"type_line,omitempty"`
	OracleText  string `json:"oracle_text,omitempty"`
	ImageNormal string `json:"image_normal,omitempty"`
	ImageSmall  string `json:"image_small,omitempty"`
}

type Card struct {
	OracleID      string            `json:"oracle_id,omitempty"`
	Name          string            `json:"name"`
	PrintedName   string            `json:"printed_name,omitempty"`
	ManaCost      string            `json:"mana_cost,omitempty"`
	TypeLine      string            `json:"type_line,omitempty"`
	OracleText    string            `json:"oracle_text,omitempty"`
	ColorIdentity []string          `json:"color_identity,omitempty"`
	Keywords      []string          `json:"keywords,omitempty"`
	Legalities    map[string]string `json:"legalities,omitempty"`
	ImageNormal   string            `json:"image_normal,omitempty"`
	ImageSmall    string            `json:"image_small,omitempty"`
	Layout        string            `json:"layout,omitempty"`
	Faces         []CardFace        `json:"faces,omitempty"`
}

type Client struct {
	baseURL    string
	httpClient *http.Client
	ttl        time.Duration
	mu         sync.RWMutex
	cache      map[string]cacheEntry
}

type cacheEntry struct {
	card      Card
	expiresAt time.Time
}

type collectionResponse struct {
	Data     []scryfallCard    `json:"data"`
	NotFound []json.RawMessage `json:"not_found"`
}

type scryfallCard struct {
	OracleID      string            `json:"oracle_id"`
	Name          string            `json:"name"`
	PrintedName   string            `json:"printed_name"`
	ManaCost      string            `json:"mana_cost"`
	TypeLine      string            `json:"type_line"`
	OracleText    string            `json:"oracle_text"`
	ColorIdentity []string          `json:"color_identity"`
	Keywords      []string          `json:"keywords"`
	Legalities    map[string]string `json:"legalities"`
	ImageURIs     map[string]string `json:"image_uris"`
	Layout        string            `json:"layout"`
	CardFaces     []struct {
		Name        string            `json:"name"`
		PrintedName string            `json:"printed_name"`
		ManaCost    string            `json:"mana_cost"`
		TypeLine    string            `json:"type_line"`
		OracleText  string            `json:"oracle_text"`
		ImageURIs   map[string]string `json:"image_uris"`
	} `json:"card_faces"`
}

func New(baseURL string, httpClient *http.Client, ttl time.Duration) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), httpClient: httpClient, ttl: ttl, cache: make(map[string]cacheEntry)}
}

func (c *Client) Lookup(ctx context.Context, names []string) (map[string]Card, error) {
	result := make(map[string]Card, len(names))
	missing := make([]string, 0, len(names))
	now := time.Now()
	for _, name := range uniqueNames(names) {
		key := normalizeName(name)
		c.mu.RLock()
		entry, ok := c.cache[key]
		c.mu.RUnlock()
		if ok && now.Before(entry.expiresAt) {
			result[key] = entry.card
			continue
		}
		missing = append(missing, name)
	}
	for start := 0; start < len(missing); start += maxBatchSize {
		end := min(start+maxBatchSize, len(missing))
		cards, err := c.fetchBatch(ctx, missing[start:end])
		if err != nil {
			return result, err
		}
		for _, card := range cards {
			for _, key := range cardLookupKeys(card) {
				result[key] = card
				c.mu.Lock()
				c.cache[key] = cacheEntry{card: card, expiresAt: time.Now().Add(c.ttl)}
				c.mu.Unlock()
			}
		}
	}
	return result, nil
}

func (c *Client) fetchBatch(ctx context.Context, names []string) ([]Card, error) {
	identifiers := make([]map[string]string, 0, len(names))
	for _, name := range names {
		identifiers = append(identifiers, map[string]string{"name": name})
	}
	body, err := json.Marshal(map[string]any{"identifiers": identifiers})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/cards/collection", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "PowerLevelAggregator/0.2")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request Scryfall: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxResponseSize {
		return nil, errors.New("Scryfall response is too large")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Scryfall returned HTTP %d", resp.StatusCode)
	}
	var payload collectionResponse
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("decode Scryfall response: %w", err)
	}
	cards := make([]Card, 0, len(payload.Data))
	for _, raw := range payload.Data {
		cards = append(cards, normalizeCard(raw))
	}
	return cards, nil
}

func normalizeCard(raw scryfallCard) Card {
	manaCost, typeLine, oracleText := raw.ManaCost, raw.TypeLine, raw.OracleText
	images := raw.ImageURIs
	faces := make([]CardFace, 0, len(raw.CardFaces))
	for _, face := range raw.CardFaces {
		faces = append(faces, CardFace{Name: face.Name, PrintedName: face.PrintedName, ManaCost: face.ManaCost, TypeLine: face.TypeLine, OracleText: face.OracleText, ImageNormal: face.ImageURIs["normal"], ImageSmall: face.ImageURIs["small"]})
	}
	if len(raw.CardFaces) > 0 {
		face := raw.CardFaces[0]
		if manaCost == "" {
			manaCost = face.ManaCost
		}
		if typeLine == "" {
			typeLine = face.TypeLine
		}
		if oracleText == "" {
			oracleText = face.OracleText
		}
		if len(images) == 0 {
			images = face.ImageURIs
		}
	}
	return Card{
		OracleID: raw.OracleID, Name: raw.Name, PrintedName: raw.PrintedName,
		ManaCost: manaCost, TypeLine: typeLine, OracleText: oracleText,
		ColorIdentity: raw.ColorIdentity, Keywords: raw.Keywords, Legalities: raw.Legalities,
		ImageNormal: images["normal"], ImageSmall: images["small"], Layout: raw.Layout, Faces: faces,
	}
}

func cardLookupKeys(card Card) []string {
	seen := make(map[string]struct{})
	var keys []string
	add := func(name string) {
		key := normalizeName(name)
		if key == "" {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	add(card.Name)
	for _, face := range card.Faces {
		add(face.Name)
	}
	if before, _, ok := strings.Cut(card.Name, " // "); ok {
		add(before)
	}
	return keys
}

func normalizeName(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func uniqueNames(names []string) []string {
	seen := make(map[string]struct{}, len(names))
	result := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		key := normalizeName(name)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, name)
	}
	return result
}

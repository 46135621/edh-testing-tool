package service

import (
	"container/list"
	"encoding/json"
	"sync"
	"time"
)

type analysisCache struct {
	mu         sync.Mutex
	entries    map[string]*list.Element
	order      *list.List
	maxEntries int
}

type cacheEntry struct {
	key       string
	analysis  Analysis
	expiresAt time.Time
}

func newAnalysisCache(maxEntries int) *analysisCache {
	if maxEntries < 1 {
		maxEntries = 1
	}
	return &analysisCache{
		entries:    make(map[string]*list.Element, maxEntries),
		order:      list.New(),
		maxEntries: maxEntries,
	}
}

func (c *analysisCache) get(key string, now time.Time) (Analysis, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.entries[key]
	if !ok {
		return Analysis{}, false
	}
	entry := element.Value.(*cacheEntry)
	if !now.Before(entry.expiresAt) {
		c.remove(element)
		return Analysis{}, false
	}
	c.order.MoveToFront(element)
	return cloneAnalysis(entry.analysis), true
}

func (c *analysisCache) set(key string, analysis Analysis, expiresAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if element, ok := c.entries[key]; ok {
		entry := element.Value.(*cacheEntry)
		entry.analysis = cloneAnalysis(analysis)
		entry.expiresAt = expiresAt
		c.order.MoveToFront(element)
		return
	}
	element := c.order.PushFront(&cacheEntry{key: key, analysis: cloneAnalysis(analysis), expiresAt: expiresAt})
	c.entries[key] = element
	for len(c.entries) > c.maxEntries {
		c.remove(c.order.Back())
	}
}

func (c *analysisCache) remove(element *list.Element) {
	if element == nil {
		return
	}
	entry := element.Value.(*cacheEntry)
	delete(c.entries, entry.key)
	c.order.Remove(element)
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(typed))
		for key, child := range typed {
			cloned[key] = cloneValue(child)
		}
		return cloned
	case []any:
		cloned := make([]any, len(typed))
		for index, child := range typed {
			cloned[index] = cloneValue(child)
		}
		return cloned
	default:
		return cloneViaJSON(value)
	}
}

func cloneViaJSON(value any) any {
	encoded, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var cloned any
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return value
	}
	return cloned
}

func cloneAnalysis(source Analysis) Analysis {
	encoded, err := json.Marshal(source)
	if err == nil {
		var cloned Analysis
		if json.Unmarshal(encoded, &cloned) == nil {
			return cloned
		}
	}
	cloned := source
	cloned.Warnings = append([]string(nil), source.Warnings...)
	cloned.Deck.Commanders = append([]string(nil), source.Deck.Commanders...)
	cloned.Results = make(map[string]ProviderResult, len(source.Results))
	for name, result := range source.Results {
		copyResult := result
		if result.Metrics != nil {
			copyResult.Metrics = make(map[string]any, len(result.Metrics))
			for key, value := range result.Metrics {
				copyResult.Metrics[key] = cloneValue(value)
			}
		}
		if result.Error != nil {
			copyError := *result.Error
			copyResult.Error = &copyError
		}
		cloned.Results[name] = copyResult
	}
	return cloned
}

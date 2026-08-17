package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Address             string
	RequestTimeout      time.Duration
	ProviderTimeout     time.Duration
	CacheTTL            time.Duration
	PartialCacheTTL     time.Duration
	CacheMaxEntries     int
	EDHMaxConcurrency   int
	ScryfallAPIURL      string
	CardCatalogTTL      time.Duration
	SpellbookAPIURL     string
	EDHRECJSONURL       string
	CommanderSaltAPIURL string
	EDHPageURL          string
	BrowserPath         string
	BrowserHeadless     bool
}

func Load() Config {
	return Config{
		Address:             env("APP_ADDRESS", ":8080"),
		RequestTimeout:      durationEnv("REQUEST_TIMEOUT", 90*time.Second),
		ProviderTimeout:     durationEnv("PROVIDER_TIMEOUT", 60*time.Second),
		CacheTTL:            durationEnv("CACHE_TTL", 30*time.Minute),
		PartialCacheTTL:     durationEnv("PARTIAL_CACHE_TTL", 45*time.Second),
		CacheMaxEntries:     intEnv("CACHE_MAX_ENTRIES", 500, 1),
		EDHMaxConcurrency:   intEnv("EDH_MAX_CONCURRENCY", 2, 1),
		ScryfallAPIURL:      env("SCRYFALL_API_URL", "https://api.scryfall.com"),
		CardCatalogTTL:      durationEnv("CARD_CATALOG_TTL", 24*time.Hour),
		SpellbookAPIURL:     env("SPELLBOOK_API_URL", "https://backend.commanderspellbook.com"),
		EDHRECJSONURL:       env("EDHREC_JSON_URL", "https://json.edhrec.com"),
		CommanderSaltAPIURL: env("COMMANDERSALT_API_URL", "https://api.commandersalt.com"),
		EDHPageURL:          env("EDH_PAGE_URL", "https://edhpowerlevel.com/"),
		BrowserPath:         env("BROWSER_PATH", ""),
		BrowserHeadless:     boolEnv("BROWSER_HEADLESS", true),
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func intEnv(key string, fallback, minimum int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < minimum {
		return fallback
	}
	return parsed
}

func boolEnv(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	if duration, err := time.ParseDuration(value); err == nil {
		return duration
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		return time.Duration(seconds) * time.Second
	}
	return fallback
}

package main

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"powerlevel/internal/api"
	"powerlevel/internal/config"
	"powerlevel/internal/providers/cardcatalog"
	"powerlevel/internal/providers/commandersalt"
	"powerlevel/internal/providers/edhpowerlevel"
	"powerlevel/internal/providers/edhrec"
	"powerlevel/internal/providers/spellbook"
	"powerlevel/internal/service"
)

//go:embed web/*
var embeddedWeb embed.FS

func main() {
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	httpClient := &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          20,
			MaxIdleConnsPerHost:   10,
			MaxConnsPerHost:       10,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: cfg.ProviderTimeout,
		},
		Timeout: cfg.ProviderTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	browserPath := cfg.BrowserPath
	if browserPath == "" {
		browserPath = edhpowerlevel.BrowserPathFromEnv()
	}
	commanderSaltClient := commandersalt.New(cfg.CommanderSaltAPIURL, httpClient)
	cardCatalogClient := cardcatalog.New(cfg.ScryfallAPIURL, httpClient, cfg.CardCatalogTTL)
	spellbookClient := spellbook.New(cfg.SpellbookAPIURL, httpClient)
	edhrecClient := edhrec.New(cfg.EDHRECJSONURL, httpClient)
	edhClient, err := edhpowerlevel.New(cfg.EDHPageURL, browserPath, cfg.BrowserHeadless, cfg.EDHMaxConcurrency)
	if err != nil {
		logger.Error("start EDH browser", "error", err)
		os.Exit(1)
	}
	defer edhClient.Close()
	analyzer := service.NewAnalyzer(
		commanderSaltClient,
		edhClient,
		cardCatalogClient,
		spellbookClient,
		edhrecClient,
		cfg.ProviderTimeout,
		cfg.RequestTimeout,
		cfg.CacheTTL,
		cfg.PartialCacheTTL,
		cfg.CacheMaxEntries,
	)

	webRoot, err := fs.Sub(embeddedWeb, "web")
	if err != nil {
		logger.Error("load embedded web files", "error", err)
		os.Exit(1)
	}
	server := &http.Server{
		Addr:              cfg.Address,
		Handler:           api.NewHandler(analyzer, logger, cfg.RequestTimeout, http.FileServer(http.FS(webRoot))),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      cfg.RequestTimeout + 5*time.Second,
		IdleTimeout:       60 * time.Second,
	}
	logger.Info("server starting", "address", cfg.Address, "browser", browserPath, "edh_max_concurrency", cfg.EDHMaxConcurrency)
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()

	signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	select {
	case <-signalCtx.Done():
		logger.Info("server shutting down")
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancelShutdown()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("graceful shutdown failed", "error", err)
			_ = server.Close()
		}
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server stopped", "error", err)
			os.Exit(1)
		}
	}
}

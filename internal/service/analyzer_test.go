package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"powerlevel/internal/deck"
	"powerlevel/internal/providers/commandersalt"
)

type blockingEDH struct {
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *blockingEDH) Analyze(ctx context.Context, _ deck.Deck) (map[string]any, error) {
	b.calls.Add(1)
	b.once.Do(func() { close(b.started) })
	select {
	case <-b.release:
		return map[string]any{"power_level": 5.5}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestAnalyzeWaiterCanCancelWithoutCancelingSharedWork(t *testing.T) {
	server := commanderSaltServer(t)
	defer server.Close()
	edh := &blockingEDH{started: make(chan struct{}), release: make(chan struct{})}
	analyzer := NewAnalyzer(
		commandersalt.New(server.URL, server.Client()), edh, nil, nil, nil,
		time.Second, 3*time.Second, time.Minute, time.Second, 10,
	)

	firstResult := make(chan error, 1)
	go func() {
		_, err := analyzer.Analyze(context.Background(), "https://www.moxfield.com/decks/example1", "example1")
		firstResult <- err
	}()
	<-edh.started

	waiterCtx, cancelWaiter := context.WithCancel(context.Background())
	cancelWaiter()
	if _, err := analyzer.Analyze(waiterCtx, "https://www.moxfield.com/decks/example1", "example1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting caller got %v, want context.Canceled", err)
	}
	close(edh.release)
	if err := <-firstResult; err != nil {
		t.Fatalf("shared work was canceled by waiter: %v", err)
	}
	if got := edh.calls.Load(); got != 1 {
		t.Fatalf("EDH called %d times, want 1", got)
	}
}

func commanderSaltServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"deckName":"Example","powerLevelRating":5,"bracketRating":3,"saltRating":10,
			"cards":{
				"commander":{"name":"Commander","count":1,"isCommander":true,"isFrontFace":true},
				"island":{"name":"Island","count":99,"isCommander":false,"isFrontFace":true}
			}
		}`))
	}))
}

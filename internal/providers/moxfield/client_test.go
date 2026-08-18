package moxfield

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoadParsesDeckZones(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/decks/all/deck123" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"Example","commanders":{"a":{"quantity":1,"card":{"name":"Commander"}}},"mainboard":{"b":{"quantity":99,"card":{"name":"Island"}}}}`))
	}))
	defer server.Close()
	result, err := New(server.URL, server.Client()).Load(context.Background(), "https://www.moxfield.com/decks/deck123", "deck123")
	if err != nil {
		t.Fatal(err)
	}
	if result.Name != "Example" || result.CardCount() != 100 || result.Commanders[0].Name != "Commander" {
		t.Fatalf("unexpected deck %+v", result)
	}
}

func TestLoadParsesMoxfieldBoardsSchema(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"Example","boards":{"commanders":{"count":1,"cards":{"a":{"quantity":1,"boardType":"commanders","card":{"name":"Commander"}}}},"mainboard":{"count":99,"cards":{"b":{"quantity":99,"boardType":"mainboard","card":{"name":"Island"}}}}}}`))
	}))
	defer server.Close()
	result, err := New(server.URL, server.Client()).Load(context.Background(), "https://www.moxfield.com/decks/deck123", "deck123")
	if err != nil {
		t.Fatal(err)
	}
	if result.CardCount() != 100 || len(result.Commanders) != 1 || result.Commanders[0].Name != "Commander" || len(result.Mainboard) != 1 || result.Mainboard[0].Quantity != 99 {
		t.Fatalf("unexpected boards deck %+v", result)
	}
}

func TestLoadClassifiesCloudflareHTML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(403)
		_, _ = w.Write([]byte("Cloudflare challenge"))
	}))
	defer server.Close()
	_, err := New(server.URL, server.Client()).Load(context.Background(), "", "deck123")
	upstream, ok := err.(*Error)
	if !ok || upstream.Code != "UPSTREAM_CHALLENGE" {
		t.Fatalf("unexpected error %v", err)
	}
}

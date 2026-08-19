package service

import (
	"testing"
)

func TestAnyColorAllowed(t *testing.T) {
	identity := map[string]struct{}{"W": {}, "U": {}}
	cases := []struct {
		colors  []string
		allowed bool
	}{
		{[]string{"W", "U"}, true},
		{[]string{"U", "R"}, true},  // off-color fetch: U in identity, R not
		{[]string{"R", "G"}, false}, // neither in identity
		{[]string{}, false},
	}
	for _, tc := range cases {
		if got := anyColorAllowed(tc.colors, identity); got != tc.allowed {
			t.Errorf("anyColorAllowed(%v, WU) = %v, want %v", tc.colors, got, tc.allowed)
		}
	}
}

func TestFilterOutSeen(t *testing.T) {
	pool := []edhrecPoolCard{
		{name: "Sol Ring"},
		{name: "Arcane Signet"},
		{name: "Rampant Growth"},
	}
	seen := map[string]struct{}{"arcane signet": {}}
	got := filterOutSeen(pool, seen)
	if len(got) != 2 {
		t.Fatalf("filterOutSeen len = %d, want 2", len(got))
	}
	for _, item := range got {
		if item.name == "Arcane Signet" {
			t.Errorf("filterOutSeen kept a seen card: %s", item.name)
		}
	}
}

func TestMergeFreshDedupes(t *testing.T) {
	base := []edhrecPoolCard{{name: "Sol Ring"}}
	fresh := []edhrecPoolCard{{name: "Sol Ring"}}
	extra := []edhrecPoolCard{{name: "Arcane Signet"}, {name: "Sol Ring"}}
	seen := map[string]struct{}{"arcane signet": {}}
	base, fresh = mergeFresh(base, fresh, extra, seen)
	if len(base) != 2 {
		t.Fatalf("mergeFresh base len = %d, want 2", len(base))
	}
	// "Arcane Signet" is in seen, so it must not enter the fresh view.
	for _, item := range fresh {
		if item.name == "Arcane Signet" {
			t.Errorf("mergeFresh added a seen card to fresh: %s", item.name)
		}
	}
	if len(fresh) != 1 || fresh[0].name != "Sol Ring" {
		t.Errorf("mergeFresh fresh = %v, want only Sol Ring", fresh)
	}
}

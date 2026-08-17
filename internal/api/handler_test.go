package api

import "testing"

func TestValidateMoxfieldURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		wantURL string
		wantID  string
		wantErr bool
	}{
		{name: "valid", input: "https://moxfield.com/decks/d8xoDMBAcUOKmd8VjkU66w", wantURL: "https://www.moxfield.com/decks/d8xoDMBAcUOKmd8VjkU66w", wantID: "d8xoDMBAcUOKmd8VjkU66w"},
		{name: "valid www and query", input: "https://www.moxfield.com/decks/d8xoDMBAcUOKmd8VjkU66w?foo=bar", wantURL: "https://www.moxfield.com/decks/d8xoDMBAcUOKmd8VjkU66w", wantID: "d8xoDMBAcUOKmd8VjkU66w"},
		{name: "reject http", input: "http://moxfield.com/decks/d8xoDMBAcUOKmd8VjkU66w", wantErr: true},
		{name: "reject suffix host", input: "https://moxfield.com.evil.test/decks/d8xoDMBAcUOKmd8VjkU66w", wantErr: true},
		{name: "reject user info", input: "https://moxfield.com@evil.test/decks/d8xoDMBAcUOKmd8VjkU66w", wantErr: true},
		{name: "reject edit path", input: "https://moxfield.com/decks/d8xoDMBAcUOKmd8VjkU66w/edit", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			gotURL, gotID, err := ValidateMoxfieldURL(test.input)
			if test.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got URL %q", gotURL)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotURL != test.wantURL || gotID != test.wantID {
				t.Fatalf("got (%q, %q), want (%q, %q)", gotURL, gotID, test.wantURL, test.wantID)
			}
		})
	}
}
